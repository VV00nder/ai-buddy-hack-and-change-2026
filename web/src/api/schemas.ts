// Схемы ответов API (Zod): интерфейс проверяет данные на границе и не падает
// от неожиданного ответа сервера.
import { z } from 'zod';

export const Role = z.enum(['intern', 'mentor', 'lead', 'hrbp', 'engineer', 'newcomer', 'admin']);
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
  email: z.string().optional(),
  registered: z.boolean().optional(),
});
export type User = z.infer<typeof User>;

export const TeamInfo = z.object({ id: z.string(), name: z.string(), leadId: z.string().optional().default(''), leadName: z.string().optional().default('') });
export type TeamInfo = z.infer<typeof TeamInfo>;
export const Department = z.object({
  members: z.array(User),
  teams: z.array(TeamInfo),
  newcomers: z.array(User).nullish().transform((v) => v ?? []),
  mentors: z.array(User).nullish().transform((v) => v ?? []),
});
export type Department = z.infer<typeof Department>;

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
  quizId: z.string().optional(),
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
  quiz: z.string().optional(),
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
  templateId: z.string().optional().default(''),
  templateName: z.string().optional().default(''),
  customized: z.boolean().optional().default(false),
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
  waitingHours: z.number().optional().default(0),
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
  templateName: z.string().optional().default(''),
  customized: z.boolean().optional().default(false),
  quizzesPassed: z.number().optional().default(0),
  quizzesFailed: z.number().optional().default(0),
});
export type InternSummary = z.infer<typeof InternSummary>;

export const GoalTask = z.object({ id: z.string(), title: z.string(), stepId: z.string().optional(), done: z.boolean() });
export type GoalTask = z.infer<typeof GoalTask>;

export const Goal = z.object({
  id: z.string(),
  internId: z.string(),
  title: z.string(),
  metric: z.string(),
  due: z.string(),
  status: z.enum(['agreed', 'in_progress', 'done']),
  progress: z.number(),
  tasks: z.array(GoalTask).nullish().transform((v) => v ?? []),
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

export const DocSummary = z.object({
  id: z.string(),
  title: z.string(),
  tags: z.array(z.string()).nullish().transform((v) => v ?? []),
  folder: z.string().optional().default(''),
  custom: z.boolean().optional().default(false),
  editable: z.boolean().optional().default(false),
});
export type DocSummary = z.infer<typeof DocSummary>;
export const Doc = DocSummary.extend({
  sections: z.array(z.object({ heading: z.string(), text: z.string() })).nullish().transform((v) => v ?? []),
  body: z.string().optional(),
  audience: z.array(z.string()).nullish().transform((v) => v ?? []),
  updatedBy: z.string().optional(),
  updatedAt: z.string().optional(),
});
export type Doc = z.infer<typeof Doc>;
export const KbList = z.object({
  docs: z.array(DocSummary),
  folders: z.array(z.string()).nullish().transform((v) => v ?? []),
});
export type KbList = z.infer<typeof KbList>;

export const Perm = z.enum(['kb', 'approvals', 'programs']);
export type Perm = z.infer<typeof Perm>;
export const QuizLock = z.object({ quizId: z.string(), title: z.string(), until: z.string() });
export type QuizLock = z.infer<typeof QuizLock>;
export const Me = z.object({
  user: User,
  perms: z.array(z.string()).nullish().transform((v) => v ?? []),
  activeQuiz: QuizLock.nullish(),
});
export type Me = z.infer<typeof Me>;

export const QuizInfo = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string().optional(),
  docIds: z.array(z.string()).nullish().transform((v) => v ?? []),
  questions: z.number(),
  passPercent: z.number(),
  builtin: z.boolean(),
  editable: z.boolean(),
  attempts: z.number(),
  best: z.number(),
  passed: z.boolean(),
});
export type QuizInfo = z.infer<typeof QuizInfo>;
export const QuizPublic = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string().optional(),
  docIds: z.array(z.string()).nullish().transform((v) => v ?? []),
  passPercent: z.number(),
  questions: z.array(z.object({ text: z.string(), options: z.array(z.string()) })),
});
export type QuizPublic = z.infer<typeof QuizPublic>;
export const QuizQuestion = z.object({
  text: z.string(),
  options: z.array(z.string()),
  correct: z.number(),
  explain: z.string().optional(),
});
export type QuizQuestion = z.infer<typeof QuizQuestion>;
export const QuizFull = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string().optional(),
  docIds: z.array(z.string()).nullish().transform((v) => v ?? []),
  passPercent: z.number(),
  questions: z.array(QuizQuestion),
  builtin: z.boolean().optional().default(false),
});
export type QuizFull = z.infer<typeof QuizFull>;
export const QuizAttempt = z.object({
  id: z.string(),
  quizId: z.string(),
  title: z.string(),
  correct: z.number(),
  total: z.number(),
  percent: z.number(),
  passed: z.boolean(),
  at: z.string(),
});
export type QuizAttempt = z.infer<typeof QuizAttempt>;
export const QuizReviewItem = z.object({
  text: z.string(),
  options: z.array(z.string()),
  chosen: z.number(),
  correct: z.number(),
  right: z.boolean(),
  explain: z.string().optional(),
  docId: z.string().optional(),
  docTitle: z.string().optional(),
});
export type QuizReviewItem = z.infer<typeof QuizReviewItem>;
export const AttemptView = QuizAttempt.extend({ review: z.array(QuizReviewItem).nullish().transform((v) => v ?? []) });
export type AttemptView = z.infer<typeof AttemptView>;
export const InternQuizResults = z.object({ intern: User, attempts: z.array(AttemptView) });
export type InternQuizResults = z.infer<typeof InternQuizResults>;
export const QuizResult = z.object({
  attempt: QuizAttempt,
  review: z.array(QuizReviewItem),
  stepsDone: z.array(z.string()).nullish().transform((v) => v ?? []),
});
export type QuizResult = z.infer<typeof QuizResult>;

export const TemplateStep = z.object({
  id: z.string(),
  period: z.string(),
  owner: z.enum(['intern', 'mentor']),
  category: z.string(),
  title: z.string(),
  description: z.string().optional().default(''),
  docs: z.array(z.string()).nullish().transform((v) => v ?? undefined),
  prompt: z.string().optional(),
  quiz: z.string().optional(),
  auto: z.object({ access: z.array(z.string()).nullish() }).nullish(),
});
export type TemplateStep = z.infer<typeof TemplateStep>;
export const Period = z.object({ id: z.string(), name: z.string(), fromDay: z.number(), toDay: z.number() });
export type Period = z.infer<typeof Period>;
export const Template = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string().optional().default(''),
  team: z.string().optional(),
  builtin: z.boolean(),
  updatedBy: z.string().optional(),
  updatedAt: z.string().optional(),
  periods: z.array(Period),
  steps: z.array(TemplateStep),
});
export type Template = z.infer<typeof Template>;
export const Program = z.object({
  internId: z.string(),
  baseId: z.string(),
  customized: z.boolean(),
  template: Template,
  updatedBy: z.string().optional(),
  updatedAt: z.string().optional(),
});
export type Program = z.infer<typeof Program>;
export const Member = z.object({ user: User, perms: z.array(z.string()) });
export type Member = z.infer<typeof Member>;

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

export const InternCard = InternSummary.extend({
  mentorName: z.string().optional().default(''),
  pendingApprovals: z.number().optional().default(0),
  waitingHours: z.number().optional().default(0),
  openRequests: z.number().optional().default(0),
  alerts: z.array(z.string()).nullish().transform((v) => v ?? []),
});
export type InternCard = z.infer<typeof InternCard>;

export const Reminder = z.object({
  id: z.string(),
  ownerId: z.string(),
  text: z.string(),
  link: z.string().optional(),
  at: z.string(),
  sent: z.boolean(),
  created: z.string(),
});
export type Reminder = z.infer<typeof Reminder>;

export const Dashboard = z.object({
  interns: z.array(InternCard),
  approvals: z.array(Approval).nullish().transform((v) => v ?? []),
  reminders: z.array(Reminder).nullish().transform((v) => v ?? []),
  newcomers: z.number().optional().default(0),
});
export type Dashboard = z.infer<typeof Dashboard>;

export const InternRequest = z.object({
  id: z.string(),
  kind: z.string(),
  title: z.string(),
  status: ActionStatus,
  externalId: z.string().optional(),
  comment: z.string().optional(),
  createdAt: z.string(),
  awaitingMe: z.boolean(),
  approval: Approval.nullish(),
});
export type InternRequest = z.infer<typeof InternRequest>;

export const Flags = z.object({
  absenceActions: z.boolean(),
  accessActions: z.boolean(),
  mentorEscalation: z.boolean(),
  faqFromAnswers: z.boolean(),
  llm: z.boolean(),
  proactive: z.boolean().optional().default(true),
});
export type Flags = z.infer<typeof Flags>;

export const AuditEntry = z.object({
  id: z.string(),
  userId: z.string(),
  userName: z.string().optional().default(''),
  type: z.string(),
  props: Dict.nullish(),
  at: z.string(),
});
export type AuditEntry = z.infer<typeof AuditEntry>;

export const ServiceInfo = z.object({
  status: z.record(z.string(), z.string()),
  mode: z.string(),
  model: z.string().optional(),
  flags: Flags,
  toggleable: z.array(z.string()).nullish().transform((v) => v ?? []),
  roles: z.record(z.string(), z.number()),
  pending: z.number(),
  newcomers: z.number(),
  eventsTotal: z.number(),
  audit: z.array(AuditEntry).nullish().transform((v) => v ?? []),
});
export type ServiceInfo = z.infer<typeof ServiceInfo>;

export const CronReport = z.object({
  digests: z.number(),
  staleRequests: z.number(),
  openQuestions: z.number(),
  reminders: z.number(),
});
export type CronReport = z.infer<typeof CronReport>;

export const InternQuestion = z.object({
  id: z.string(),
  text: z.string(),
  status: z.string(),
  answer: z.string().optional(),
  mentorName: z.string().optional(),
  createdAt: z.string(),
});
export type InternQuestion = z.infer<typeof InternQuestion>;
