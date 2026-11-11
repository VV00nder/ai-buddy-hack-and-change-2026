// HTTP-клиент API AI-buddy. Все ответы проходят через Zod-схемы.
import { z } from 'zod';
import * as S from './schemas';

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

type FetchLike = typeof fetch;

export function createApi(fetchImpl: FetchLike = (...a) => fetch(...a), base = '') {
  async function call<T extends z.ZodTypeAny>(method: string, path: string, schema: T, body?: unknown): Promise<z.infer<T>> {
    let resp: Response;
    try {
      resp = await fetchImpl(base + path, {
        method,
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'ai-buddy' },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch {
      throw new ApiError(0, 'network', 'Нет связи с сервером. Проверь подключение и повтори.');
    }
    const data: unknown = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      const e = (data ?? {}) as { error?: string; message?: string };
      throw new ApiError(resp.status, e.error ?? 'error', e.message ?? `Ошибка ${resp.status}`);
    }
    const parsed = schema.safeParse(data);
    if (!parsed.success) {
      throw new ApiError(resp.status, 'schema', 'Сервер вернул неожиданный ответ');
    }
    return parsed.data;
  }
  const ok = z.object({}).passthrough();
  const obj = z.object;
  return {
    config: () => call('GET', '/api/config', S.Config),
    demoUsers: () => call('GET', '/api/auth/demo-users', obj({ users: S.User.array(), password: z.string() })),
    login: (login: string, password: string) => call('POST', '/api/auth/login', obj({ user: S.User }), { login, password }),
    logout: () => call('POST', '/api/auth/logout', ok),
    me: () => call('GET', '/api/me', obj({ user: S.User })),
    chat: () => call('GET', '/api/messages', S.ChatState),
    send: (text: string) => call('POST', '/api/messages', obj({ user: S.Message, assistant: S.Message }), { text }),
    feedback: (id: string, helpful: boolean) => call('POST', `/api/messages/${encodeURIComponent(id)}/feedback`, ok, { helpful }),
    actions: () => call('GET', '/api/actions', obj({ actions: S.Action.array().nullish().transform((v) => v ?? []) })),
    confirm: (id: string, edits: Record<string, string> = {}) =>
      call('POST', `/api/actions/${encodeURIComponent(id)}/confirm`, obj({ action: S.Action }), { edits }),
    cancel: (id: string) => call('POST', `/api/actions/${encodeURIComponent(id)}/cancel`, obj({ action: S.Action })),
    plan: () => call('GET', '/api/plan', S.Plan),
    setStep: (stepId: string, status: S.StepStatus, internId?: string, note?: string) =>
      call('PATCH', `/api/plan/steps/${encodeURIComponent(stepId)}`, S.Plan, { status, internId, note }),
    goals: (internId?: string) =>
      call('GET', `/api/goals${internId ? `?internId=${encodeURIComponent(internId)}` : ''}`, obj({ goals: S.Goal.array() })),
    addGoal: (internId: string, title: string, metric: string, due: string) =>
      call('POST', '/api/goals', obj({ goal: S.Goal }), { internId, title, metric, due }),
    updateGoal: (id: string, patch: { status?: S.Goal['status']; progress?: number; note?: string }) =>
      call('PATCH', `/api/goals/${encodeURIComponent(id)}`, obj({ goal: S.Goal }), patch),
    team: () => call('GET', '/api/team', obj({ interns: S.InternSummary.array() })),
    internPlan: (id: string) => call('GET', `/api/team/${encodeURIComponent(id)}/plan`, obj({ plan: S.Plan, intern: S.User })),
    approvals: () => call('GET', '/api/approvals', obj({ approvals: S.Approval.array() })),
    decide: (kind: string, id: string, approve: boolean, comment: string) =>
      call('POST', `/api/approvals/${encodeURIComponent(kind)}/${encodeURIComponent(id)}`, ok, { approve, comment }),
    inbox: () => call('GET', '/api/inbox', obj({ questions: S.Question.array() })),
    answer: (id: string, answer: string, addToFaq: boolean) =>
      call('POST', `/api/inbox/${encodeURIComponent(id)}/answer`, obj({ question: S.Question }), { answer, addToFaq }),
    notifications: () => call('GET', '/api/notifications', obj({ notifications: S.Notification.array() })),
    readNotifications: () => call('POST', '/api/notifications/read', ok),
    insights: () => call('GET', '/api/insights', S.Insights),
    docs: () => call('GET', '/api/kb', obj({ docs: S.DocSummary.array() })),
    doc: (id: string) => call('GET', `/api/kb/${encodeURIComponent(id)}`, S.Doc),
  };
}

export type Api = ReturnType<typeof createApi>;
export const api = createApi();
