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
    register: (r: RegisterInput) => call('POST', '/api/auth/register', obj({ user: S.User }), r),
    authTeams: () => call('GET', '/api/auth/teams', obj({ teams: S.TeamInfo.array() })),
    department: () => call('GET', '/api/department', S.Department),
    transfer: (userId: string, team: string) => call('POST', `/api/department/${encodeURIComponent(userId)}/transfer`, S.Department, { team }),
    me: () => call('GET', '/api/me', S.Me),
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
    addGoal: (internId: string, g: GoalInput) => call('POST', '/api/goals', obj({ goal: S.Goal }), { internId, ...g }),
    updateGoal: (id: string, patch: { note?: string }) => call('PATCH', `/api/goals/${encodeURIComponent(id)}`, obj({ goal: S.Goal }), patch),
    setGoalTask: (goalId: string, taskId: string, done: boolean) =>
      call('PATCH', `/api/goals/${encodeURIComponent(goalId)}/tasks/${encodeURIComponent(taskId)}`, obj({ goal: S.Goal }), { done }),
    addGoalTask: (goalId: string, t: TaskInput) => call('POST', `/api/goals/${encodeURIComponent(goalId)}/tasks`, obj({ goal: S.Goal }), t),
    dashboard: () => call('GET', '/api/dashboard', S.Dashboard),
    internRequests: (id: string) =>
      call(
        'GET',
        `/api/team/${encodeURIComponent(id)}/requests`,
        obj({
          requests: S.InternRequest.array(),
          questions: S.InternQuestion.array().nullish().transform((v) => v ?? []),
          canDelete: z.boolean().optional().default(false),
        }),
      ),
    deleteRequest: (internId: string, id: string) =>
      call('DELETE', `/api/team/${encodeURIComponent(internId)}/requests/${encodeURIComponent(id)}`, ok),
    deleteQuestion: (internId: string, id: string) =>
      call('DELETE', `/api/team/${encodeURIComponent(internId)}/questions/${encodeURIComponent(id)}`, ok),
    reminders: () => call('GET', '/api/reminders', obj({ reminders: S.Reminder.array() })),
    addReminder: (r: { text: string; link?: string; inHours?: number; at?: string }) => call('POST', '/api/reminders', S.Reminder, r),
    deleteReminder: (id: string) => call('DELETE', `/api/reminders/${encodeURIComponent(id)}`, ok),
    assign: (userId: string, role: string, mentorId: string, team?: string) =>
      call('POST', `/api/department/${encodeURIComponent(userId)}/assign`, S.Department, { role, mentorId, team }),
    adminService: () => call('GET', '/api/admin/service', S.ServiceInfo),
    adminFlag: (name: string, enabled: boolean) => call('PUT', `/api/admin/flags/${encodeURIComponent(name)}`, obj({ flags: S.Flags }), { enabled }),
    adminCron: () => call('POST', '/api/admin/cron', S.CronReport),
    adminSetUser: (userId: string, v: { role: string; team: string; mentorId?: string }) =>
      call('PATCH', `/api/admin/users/${encodeURIComponent(userId)}`, S.Department, v),
    adminPassword: (userId: string, password: string) =>
      call('POST', `/api/admin/users/${encodeURIComponent(userId)}/password`, ok, { password }),
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
    docs: (q = '') => call('GET', `/api/kb${q.trim() ? `?q=${encodeURIComponent(q.trim())}` : ''}`, S.KbList),
    doc: (id: string) => call('GET', `/api/kb/${encodeURIComponent(id)}`, S.Doc),
    saveDoc: (id: string | null, d: DocInput) =>
      id ? call('PUT', `/api/kb/${encodeURIComponent(id)}`, S.Doc, d) : call('POST', '/api/kb', S.Doc, d),
    deleteDoc: (id: string) => call('DELETE', `/api/kb/${encodeURIComponent(id)}`, ok),
    createFolder: (path: string) => call('POST', '/api/kb/folders', obj({ folders: z.array(z.string()) }), { path }),
    deleteFolder: (path: string) => call('DELETE', `/api/kb/folders?path=${encodeURIComponent(path)}`, obj({ folders: z.array(z.string()) })),
    quizzes: () => call('GET', '/api/quizzes', obj({ quizzes: S.QuizInfo.array() })),
    quiz: (id: string) => call('GET', `/api/quizzes/${encodeURIComponent(id)}`, S.QuizPublic),
    startQuiz: (id: string) => call('POST', `/api/quizzes/${encodeURIComponent(id)}/start`, S.QuizPublic),
    cancelQuiz: () => call('DELETE', '/api/quizzes/active', ok),
    quizFull: (id: string) => call('GET', `/api/quizzes/${encodeURIComponent(id)}/edit`, S.QuizFull),
    saveQuiz: (id: string | null, q: QuizInput) =>
      id ? call('PUT', `/api/quizzes/${encodeURIComponent(id)}`, S.QuizFull, q) : call('POST', '/api/quizzes', S.QuizFull, q),
    deleteQuiz: (id: string) => call('DELETE', `/api/quizzes/${encodeURIComponent(id)}`, ok),
    submitQuiz: (id: string, answers: number[]) => call('POST', `/api/quizzes/${encodeURIComponent(id)}/submit`, S.QuizResult, { answers }),
    quizResults: (internId?: string) =>
      call('GET', `/api/quiz-results${internId ? `?internId=${encodeURIComponent(internId)}` : ''}`, obj({ attempts: S.AttemptView.array() })),
    teamQuizResults: () => call('GET', '/api/quiz-results/team', obj({ interns: S.InternQuizResults.array() })),
    templates: () => call('GET', '/api/templates', obj({ templates: S.Template.array() })),
    saveTemplate: (t: TemplateInput) =>
      t.id ? call('PUT', `/api/templates/${encodeURIComponent(t.id)}`, S.Template, t) : call('POST', '/api/templates', S.Template, t),
    deleteTemplate: (id: string) => call('DELETE', `/api/templates/${encodeURIComponent(id)}`, ok),
    program: (internId: string) => call('GET', `/api/team/${encodeURIComponent(internId)}/program`, S.Program),
    saveProgram: (internId: string, steps: S.TemplateStep[]) =>
      call('PUT', `/api/team/${encodeURIComponent(internId)}/program`, S.Program, { steps }),
    assignTemplate: (internId: string, templateId: string) =>
      call('POST', `/api/team/${encodeURIComponent(internId)}/program/assign`, S.Program, { templateId }),
    rights: () => call('GET', '/api/rights', obj({ members: S.Member.array() })),
    setRight: (userId: string, perm: string, enabled: boolean) => call('POST', '/api/rights', obj({ members: S.Member.array() }), { userId, perm, enabled }),
  };
}

export interface RegisterInput {
  name: string;
  email: string;
  password: string;
}

export interface TaskInput {
  title: string;
  stepId?: string;
}

export interface GoalInput {
  title: string;
  metric: string;
  due: string;
  tasks: TaskInput[];
}

export interface DocInput {
  title: string;
  folder: string;
  audience: string[];
  tags: string[];
  body: string;
}

export interface QuizInput {
  title: string;
  description?: string;
  docIds: string[];
  passPercent: number;
  questions: S.QuizQuestion[];
}

export interface TemplateInput {
  id?: string;
  name: string;
  description: string;
  periods: S.Period[];
  steps: S.TemplateStep[];
}

export type Api = ReturnType<typeof createApi>;
export const api = createApi();
