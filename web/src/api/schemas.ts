// Схемы ответов API (Zod): интерфейс проверяет данные на границе и не падает
// от неожиданного ответа сервера.
import { z } from 'zod';

export const Role = z.enum(['intern', 'mentor', 'lead', 'hrbp', 'engineer']);
export type Role = z.infer<typeof Role>;

export const User = z.object({
  id: z.string(),
  login: z.string(),
  name: z.string(),
  role: Role,
  team: z.string(),
  teamName: z.string().optional().default(''),
  track: z.string().optional(),
  mentorId: z.string().optional(),
  leadId: z.string().optional(),
  startDate: z.string(),
});
export type User = z.infer<typeof User>;

export const Source = z.object({ docId: z.string(), title: z.string(), section: z.string().optional() });
export type Source = z.infer<typeof Source>;

export const TraceItem = z.object({ kind: z.string(), label: z.string(), at: z.string() });

export const Message = z.object({
  id: z.string(),
  userId: z.string(),
  author: z.enum(['user', 'assistant']),
  text: z.string(),
  status: z.enum(['pending', 'done', 'error']),
  sources: z.array(Source).nullish().transform((v) => v ?? []),
  actionId: z.string().optional(),
  suggestions: z.array(z.string()).nullish().transform((v) => v ?? []),
  trace: z.array(TraceItem).nullish().transform((v) => v ?? []),
  mode: z.string().optional(),
  feedback: z.boolean().optional(),
  createdAt: z.string(),
  updatedAt: z.string().optional(),
});
export type Message = z.infer<typeof Message>;

export const ActionStatus = z.enum(['proposed', 'submitted', 'approved', 'rejected', 'cancelled', 'failed']);
export type ActionStatus = z.infer<typeof ActionStatus>;

const Dict = z.record(z.string(), z.unknown());

export const Action = z.object({
  id: z.string(),
  userId: z.string(),
  kind: z.enum(['absence', 'access', 'mentor_question']),
  tool: z.string(),
  title: z.string(),
  args: Dict.nullish().transform((v) => v ?? {}),
  preview: Dict.nullish().transform((v) => v ?? {}),
  status: ActionStatus,
  externalId: z.string().optional(),
  result: Dict.nullish().transform((v) => v ?? {}),
  error: z.string().optional(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type Action = z.infer<typeof Action>;

export const ChatState = z.object({
  messages: z.array(Message),
  actions: z.array(Action).nullish().transform((v) => v ?? []),
});
export type ChatState = z.infer<typeof ChatState>;

export const StepStatus = z.enum(['todo', 'in_progress', 'done', 'blocked']);
export type StepStatus = z.infer<typeof StepStatus>;

export const StepView = z.object({
  id: z.string(),
  period: z.string(),
  owner: z.enum(['intern', 'mentor']),
  category: z.string(),
  title: z.string(),
  description: z.string(),
  docs: z.array(z.string()).nullish(),
  prompt: z.string().optional(),
  status: StepStatus,
  note: z.string().optional(),
  due: z.string(),
  overdue: z.boolean(),
});
export type StepView = z.infer<typeof StepView>;

export const PeriodView = z.object({
  id: z.string(),
  name: z.string(),
  state: z.enum(['past', 'current', 'future']),
  done: z.number(),
  total: z.number(),
  steps: z.array(StepView),
});
export type PeriodView = z.infer<typeof PeriodView>;

export const Plan = z.object({
  userId: z.string(),
  day: z.number(),
  currentPeriod: z.string(),
  done: z.number(),
  total: z.number(),
  percent: z.number(),
  overdue: z.number(),
  next: StepView.nullish(),
  periods: z.array(PeriodView),
});
export type Plan = z.infer<typeof Plan>;

export const Notification = z.object({
  id: z.string(),
  kind: z.string(),
  text: z.string(),
  link: z.string().optional(),
  read: z.boolean(),
  createdAt: z.string(),
});
export type Notification = z.infer<typeof Notification>;

export const Question = z.object({
  id: z.string(),
  internId: z.string(),
  mentorId: z.string(),
  text: z.string(),
  answer: z.string().optional(),
  addToFaq: z.boolean().optional(),
  status: z.enum(['open', 'answered']),
  createdAt: z.string(),
});
export type Question = z.infer<typeof Question>;

export const Approval = z.object({
  kind: z.enum(['absence', 'access']),
  externalId: z.string(),
  userId: z.string(),
  userName: z.string(),
  title: z.string(),
  details: Dict,
});
export type Approval = z.infer<typeof Approval>;

export const InternSummary = z.object({
  user: User,
  day: z.number(),
  percent: z.number(),
  done: z.number(),
  total: z.number(),
  overdue: z.number(),
  nextStep: z.string().optional(),
  openQuestions: z.number(),
  goalsDone: z.number().optional().default(0),
  goalsTotal: z.number().optional().default(0),
});
export type InternSummary = z.infer<typeof InternSummary>;

export const Goal = z.object({
  id: z.string(),
  internId: z.string(),
  title: z.string(),
  metric: z.string(),
  due: z.string(),
  status: z.enum(['agreed', 'in_progress', 'done']),
  progress: z.number(),
  note: z.string().optional(),
  createdBy: z.string(),
  createdAt: z.string(),
  updatedAt: z.string().optional(),
});
export type Goal = z.infer<typeof Goal>;

export const Insights = z.object({
  questions: z.number(),
  answeredByBuddy: z.number(),
  selfServeRate: z.number(),
  escalations: z.number(),
  mentorAnswers: z.number(),
  actionsSubmitted: z.number(),
  actionsCompleted: z.number(),
  helpfulRate: z.number(),
  feedbacks: z.number(),
  avgAnswerSeconds: z.number(),
  stepsCompleted: z.number(),
  mentorMinutesSaved: z.number(),
  byDay: z.array(z.object({ day: z.string(), questions: z.number(), escalations: z.number() })).nullish().transform((v) => v ?? []),
});
export type Insights = z.infer<typeof Insights>;

export const DocSummary = z.object({ id: z.string(), title: z.string(), tags: z.array(z.string()).nullish() });
export const Doc = DocSummary.extend({
  sections: z.array(z.object({ heading: z.string(), text: z.string() })).nullish().transform((v) => v ?? []),
});
export type Doc = z.infer<typeof Doc>;

export const Config = z.object({
  mode: z.enum(['llm', 'offline']),
  flags: z.object({
    absenceActions: z.boolean(),
    accessActions: z.boolean(),
    mentorEscalation: z.boolean(),
    faqFromAnswers: z.boolean(),
    llm: z.boolean(),
  }),
});
export type Config = z.infer<typeof Config>;

export const Update = z.object({ type: z.string(), data: z.unknown() });
