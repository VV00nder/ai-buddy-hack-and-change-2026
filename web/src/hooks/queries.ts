// Данные сервера через TanStack Query + обновления из SSE.
import { QueryClient, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { api, ApiError } from '../api/client';
import * as S from '../api/schemas';
import { subscribe } from '../api/sse';
import { useUi } from '../store/ui';

export const keys = {
  me: ['me'] as const,
  config: ['config'] as const,
  chat: ['chat'] as const,
  plan: ['plan'] as const,
  notifications: ['notifications'] as const,
  approvals: ['approvals'] as const,
  inbox: ['inbox'] as const,
  team: ['team'] as const,
  insights: ['insights'] as const,
  internPlan: (id: string) => ['internPlan', id] as const,
  docs: (q = '') => ['docs', q] as const,
  quizzes: ['quizzes'] as const,
  quiz: (id: string) => ['quiz', id] as const,
  quizFull: (id: string) => ['quizFull', id] as const,
  quizResults: (internId = '') => ['quizResults', internId] as const,
  teamQuizResults: ['teamQuizResults'] as const,
  templates: ['templates'] as const,
  program: (internId: string) => ['program', internId] as const,
  rights: ['rights'] as const,
  department: ['department'] as const,
  goals: (internId = '') => ['goals', internId] as const,
  doc: (id: string) => ['doc', id] as const,
};

export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (n, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && n < 2,
        staleTime: 10_000,
        refetchOnWindowFocus: false,
      },
    },
  });
}

export function upsertBy<T extends { id: string }>(list: T[], item: T): T[] {
  const i = list.findIndex((x) => x.id === item.id);
  if (i === -1) return [...list, item];
  const next = list.slice();
  next[i] = item;
  return next;
}

/** upsertNewer — как upsertBy, но не заменяет более свежую версию записи
 *  (события SSE могут прийти раньше ответа на POST). */
export function upsertNewer<T extends { id: string; updatedAt?: string; status?: string }>(list: T[], item: T): T[] {
  const cur = list.find((x) => x.id === item.id);
  if (cur) {
    const curT = Date.parse(cur.updatedAt ?? '') || 0;
    const newT = Date.parse(item.updatedAt ?? '') || 0;
    if (curT > newT || (curT === newT && cur.status !== 'pending' && item.status === 'pending')) return list;
  }
  return upsertBy(list, item);
}

export function byCreated<T extends { createdAt: string }>(list: T[]): T[] {
  // при равных миллисекундах сравниваем исходные строки RFC3339 с наносекундами
  return list.slice().sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt) || (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0));
}

/** applyUpdate — применяет событие сервера к кэшу запросов. */
export function applyUpdate(qc: QueryClient, u: { type: string; data?: unknown }) {
  switch (u.type) {
    case 'message': {
      const m = S.Message.safeParse(u.data);
      if (m.success) qc.setQueryData<S.ChatState>(keys.chat, (s) => (s ? { ...s, messages: upsertNewer(s.messages, m.data) } : s));
      break;
    }
    case 'action': {
      const a = S.Action.safeParse(u.data);
      if (a.success) qc.setQueryData<S.ChatState>(keys.chat, (s) => (s ? { ...s, actions: upsertNewer(s.actions, a.data) } : s));
      break;
    }
    case 'notification': {
      const n = S.Notification.safeParse(u.data);
      if (n.success) {
        qc.setQueryData<{ notifications: S.Notification[] }>(keys.notifications, (s) => ({ notifications: [n.data, ...(s?.notifications ?? [])] }));
        // права и программы меняют руководитель и наставник — обновляем зависящие от них данные
        if (n.data.kind === 'rights') void qc.invalidateQueries({ queryKey: keys.me });
        // перевод в другой отдел или новый сотрудник: профиль, план и команда меняются
        if (n.data.kind === 'team') {
          void qc.invalidateQueries({ queryKey: keys.me });
          void qc.invalidateQueries({ queryKey: keys.plan });
          void qc.invalidateQueries({ queryKey: keys.team });
          void qc.invalidateQueries({ queryKey: keys.department });
          void qc.invalidateQueries({ queryKey: keys.approvals });
        }
        if (n.data.kind === 'quiz') {
          void qc.invalidateQueries({ queryKey: keys.team });
          void qc.invalidateQueries({ queryKey: ['quizResults'] });
          void qc.invalidateQueries({ queryKey: keys.teamQuizResults });
        }
      }
      break;
    }
    case 'plan': {
      const p = S.Plan.safeParse(u.data);
      if (p.success) qc.setQueryData(keys.plan, p.data);
      break;
    }
    case 'goal':
      void qc.invalidateQueries({ queryKey: ['goals'] });
      break;
    case 'approval':
      void qc.invalidateQueries({ queryKey: keys.approvals });
      break;
    case 'question':
      void qc.invalidateQueries({ queryKey: keys.inbox });
      void qc.invalidateQueries({ queryKey: keys.team });
      break;
  }
}

export function useLiveUpdates(enabled: boolean) {
  const qc = useQueryClient();
  const setOnline = useUi((s) => s.setOnline);
  useEffect(() => {
    if (!enabled || typeof EventSource === 'undefined') return;
    return subscribe((u) => applyUpdate(qc, u), setOnline);
  }, [enabled, qc, setOnline]);
}

export const useMe = () => useQuery({ queryKey: keys.me, queryFn: api.me, retry: false });

/** usePerms — права текущего пользователя (инструкции, согласования, программы). */
export function usePerms(): Set<string> {
  const me = useMe();
  return new Set(me.data?.perms ?? []);
}
export const useConfig = () => useQuery({ queryKey: keys.config, queryFn: api.config, staleTime: Infinity });

export function useChat() {
  const online = useUi((s) => s.online);
  return useQuery({
    queryKey: keys.chat,
    queryFn: api.chat,
    // если поток обновлений недоступен — опрашиваем, пока есть незавершённые ответы
    refetchInterval: (q) => (!online && q.state.data?.messages.some((m) => m.status === 'pending') ? 1500 : false),
  });
}

export const usePlan = () => useQuery({ queryKey: keys.plan, queryFn: api.plan });
export const useNotifications = () => useQuery({ queryKey: keys.notifications, queryFn: api.notifications });

export function useSend() {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  return useMutation({
    mutationFn: api.send,
    onSuccess: ({ user, assistant }) => {
      qc.setQueryData<S.ChatState>(keys.chat, (s) => {
        const base = s ?? { messages: [], actions: [] };
        return { ...base, messages: upsertNewer(upsertNewer(base.messages, user), assistant) };
      });
    },
    onError: (e) => toast(e instanceof Error ? e.message : 'Не удалось отправить', 'error'),
  });
}

function useActionMutation<A>(fn: (a: A) => Promise<{ action: S.Action }>, done: (a: S.Action) => string) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  return useMutation({
    mutationFn: fn,
    onSuccess: ({ action }) => {
      qc.setQueryData<S.ChatState>(keys.chat, (s) => (s ? { ...s, actions: upsertNewer(s.actions, action) } : s));
      void qc.invalidateQueries({ queryKey: keys.plan });
      toast(done(action), action.status === 'failed' ? 'error' : 'info');
    },
    onError: (e) => toast(e instanceof Error ? e.message : 'Ошибка', 'error'),
  });
}

export const useConfirm = () =>
  useActionMutation(
    (v: { id: string; edits?: Record<string, string> }) => api.confirm(v.id, v.edits),
    (a) =>
      a.status === 'failed'
        ? `Не выполнено: ${a.error ?? 'сервис отклонил заявку'}`
        : a.kind === 'mentor_question'
          ? 'Вопрос отправлен наставнику'
          : a.status === 'approved'
            ? 'Готово — доступ выдан'
            : 'Заявка отправлена на согласование',
  );

export const useCancel = () => useActionMutation((id: string) => api.cancel(id), () => 'Черновик отменён');

export function useSetStep() {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  return useMutation({
    mutationFn: (v: { stepId: string; status: S.StepStatus; internId?: string; note?: string }) => api.setStep(v.stepId, v.status, v.internId, v.note),
    onSuccess: (plan, v) => {
      if (v.internId) qc.setQueryData(keys.internPlan(v.internId), (old: { plan: S.Plan; intern: S.User } | undefined) => (old ? { ...old, plan } : old));
      else qc.setQueryData(keys.plan, plan);
      void qc.invalidateQueries({ queryKey: keys.team });
    },
    onError: (e) => toast(e instanceof Error ? e.message : 'Ошибка', 'error'),
  });
}

export function useFeedback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; helpful: boolean }) => api.feedback(v.id, v.helpful),
    onMutate: (v) =>
      qc.setQueryData<S.ChatState>(keys.chat, (s) =>
        s ? { ...s, messages: s.messages.map((m) => (m.id === v.id ? { ...m, feedback: v.helpful } : m)) } : s,
      ),
  });
}
