import assert from 'node:assert/strict';
import { test } from 'node:test';
import { QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { App, Root } from '../App';
import type { Action, InternCard } from '../api/schemas';
import { keys, makeQueryClient } from '../hooks/queries';
import { uiStore, useUi, type View } from '../store/ui';
import { absence, anna, insights, maria, msgs, oleg, plan } from '../test-fixtures';
import { ActionCard, editableFields, facts } from './ActionCard';
import { ApprovalRow, Approvals, RejectForm } from './Approvals';
import { Bubble, Chat } from './Chat';
import { AnswerForm, Inbox } from './Inbox';
import { Insights, InsightsBody } from './Insights';
import { buildTree, DocEditor, DocView, FolderForm, Knowledge } from './Knowledge';
import { ProgramPanel, Programs, StepEditor, TemplateEditor } from './Programs';
import { AttemptAnswers, QuizResultsPanel, TeamQuizResults } from './QuizResults';
import { QuizEditor, QuizForm, QuizReview, QuizRunner, Quizzes } from './Quizzes';
import { AdminDepartment, Department, Rights } from './Rights';
import { eventText, Service } from './Service';
import { Register } from './Login';
import { Waiting } from './Waiting';
import { Login } from './Login';
import { PlanBody, PlanView } from './PlanView';
import { Requests } from './Requests';
import { navFor, Shell } from './Shell';
import { DeleteButton, filterInterns, InternFilters, InternRequests, MentorsPanel, Team } from './Team';
import { TodayPanel } from './TodayPanel';

// В SSR Zustand читает getInitialState; в тестах рендерим текущее состояние стора.
uiStore.getInitialState = uiStore.getState;

function seeded() {
  const qc = makeQueryClient();
  qc.setQueryData(keys.me, { user: anna });
  qc.setQueryData(keys.config, { mode: 'offline', flags: { absenceActions: true, accessActions: true, mentorEscalation: true, faqFromAnswers: true, llm: true } });
  qc.setQueryData(keys.chat, { messages: msgs, actions: [absence] });
  qc.setQueryData(keys.plan, plan);
  qc.setQueryData(keys.notifications, { notifications: [{ id: 'n1', kind: 'action', text: 'Заявка отправлена', read: false, createdAt: '2026-11-10T12:00:00Z' }] });
  qc.setQueryData(keys.insights, insights);
  qc.setQueryData(keys.approvals, {
    approvals: [{ kind: 'absence', externalId: 'ABS-1', userId: 'u-anna', userName: 'Анна Смирнова', title: 'Отгул 13.11', details: { reason: 'учёба' } }],
  });
  qc.setQueryData(keys.inbox, {
    questions: [
      { id: 'q1', internId: 'u-anna', mentorId: 'u-oleg', text: 'Где стенд?', status: 'open', createdAt: '2026-11-10T12:00:00Z' },
      { id: 'q2', internId: 'u-anna', mentorId: 'u-oleg', text: 'Где логи?', answer: 'В Observer', addToFaq: true, status: 'answered', createdAt: '2026-11-10T12:00:00Z' },
    ],
  });
  qc.setQueryData(keys.team, { interns: [
    { user: anna, day: 4, percent: 33, done: 1, total: 3, overdue: 1, nextStep: 'Stash', openQuestions: 1, goalsDone: 1, goalsTotal: 2 },
    { user: { ...anna, id: 'u-x' }, day: 30, percent: 100, done: 3, total: 3, overdue: 0, openQuestions: 0, goalsDone: 0, goalsTotal: 0 },
  ] });
  qc.setQueryData(keys.internPlan('u-anna'), { plan, intern: anna });
  qc.setQueryData(keys.docs(''), {
    docs: [
      { id: 'process-day-off', title: 'Как оформить отгул', tags: [], folder: 'Процессы/Отсутствия', custom: false, editable: false },
      { id: 'doc_1', title: 'Релизы поиска', tags: [], folder: 'Процессы/Релизы', custom: true, editable: true },
    ],
    folders: ['Процессы', 'Процессы/Отсутствия', 'Процессы/Релизы', 'Пустая'],
  });
  qc.setQueryData(keys.doc('process-day-off'), { id: 'process-day-off', title: 'Как оформить отгул', tags: [], folder: 'Процессы/Отсутствия', custom: false, editable: false, audience: [], sections: [{ heading: 'Отгул', text: '- шаг' }] });
  qc.setQueryData(keys.doc('doc_1'), { id: 'doc_1', title: 'Релизы поиска', tags: ['релиз'], folder: 'Процессы/Релизы', custom: true, editable: true, audience: ['intern'], body: 'По вторникам', updatedBy: 'Мария', sections: [{ heading: 'Релизы поиска', text: 'По вторникам' }] });
  qc.setQueryData(keys.quizzes, {
    quizzes: [
      { id: 'quiz-dayoff', title: 'Отгулы', description: 'Коротко', docIds: ['process-day-off'], questions: 3, passPercent: 67, builtin: true, editable: false, attempts: 1, best: 100, passed: true },
      { id: 'quiz_2', title: 'Релизы', docIds: ['doc_1'], questions: 1, passPercent: 80, builtin: false, editable: true, attempts: 1, best: 0, passed: false },
      { id: 'quiz_3', title: 'Новый', docIds: [], questions: 2, passPercent: 80, builtin: false, editable: false, attempts: 0, best: 0, passed: false },
    ],
  });
  qc.setQueryData(keys.quiz('quiz-dayoff'), { id: 'quiz-dayoff', title: 'Отгулы', docIds: ['process-day-off'], passPercent: 67, questions: [{ text: 'Кто согласует?', options: ['Наставник', 'Руководитель'] }] });
  qc.setQueryData(keys.quizFull('quiz_2'), { id: 'quiz_2', title: 'Релизы', docIds: ['doc_1'], passPercent: 80, questions: [{ text: 'Когда?', options: ['Вт', 'Ср', 'Чт'], correct: 0, explain: 'Так' }] });
  const tplStep = (id: string, period: string, extra = {}) => ({ id, period, owner: 'intern', category: 'Доступы', title: `Шаг ${id}`, description: 'd', ...extra });
  const periods = [{ id: 'week1', name: '1 неделя', fromDay: 0, toDay: 6 }, { id: 'week2', name: '2 неделя', fromDay: 7, toDay: 13 }];
  const tpl = { id: 'intern-dev', name: 'Стажёр-разработчик', description: 'Базовый', builtin: true, periods, steps: [tplStep('s1', 'week1'), tplStep('s2', 'week1', { owner: 'mentor' }), tplStep('s3', 'week2', { quiz: 'quiz-dayoff' })] };
  qc.setQueryData(keys.templates, { templates: [tpl, { ...tpl, id: 'tpl_1', name: 'Аналитик поиска', builtin: false, team: 'search', updatedBy: 'Олег' }] });
  qc.setQueryData(keys.program('u-anna'), { internId: 'u-anna', baseId: 'tpl_1', customized: true, template: { ...tpl, id: 'tpl_1', builtin: false }, updatedBy: 'Мария' });
  const rv = (right: boolean) => ({ text: 'Кто согласует отгул?', options: ['Наставник', 'Руководитель'], chosen: right ? 1 : 0, correct: 1, right });
  const attempts = [
    { id: 'a1', quizId: 'quiz-dayoff', title: 'Отгулы', correct: 1, total: 3, percent: 33, passed: false, at: '2026-11-10T12:00:00Z', review: [rv(false)] },
    { id: 'a2', quizId: 'quiz-dayoff', title: 'Отгулы', correct: 3, total: 3, percent: 100, passed: true, at: '2026-11-10T13:00:00Z', review: [rv(true)] },
    { id: 'a3', quizId: 'q-old', title: 'Удалённый', correct: 0, total: 1, percent: 0, passed: false, at: '2026-11-10T14:00:00Z', review: [] },
  ];
  qc.setQueryData(keys.quizResults('u-anna'), { attempts });
  qc.setQueryData(keys.teamQuizResults, { interns: [{ intern: anna, attempts }, { intern: { ...anna, id: 'u-x', name: 'Иван' }, attempts: [] }] });
  qc.setQueryData(['auth-teams'], { teams: [{ id: 'search', name: 'Поиск объявлений', leadId: 'u-maria', leadName: 'Мария Волкова' }] });
  qc.setQueryData(keys.department, {
    members: [anna, { ...oleg, email: 'oleg@example.com', registered: true }],
    newcomers: [{ ...anna, id: 'u-new', name: 'Пётр Сидоров', role: 'newcomer', team: '', email: 'petr@example.com', registered: true }],
    mentors: [oleg],
    teams: [{ id: 'search', name: 'Поиск объявлений', leadId: 'u-maria', leadName: 'Мария Волкова' }, { id: 'delivery', name: 'Доставка', leadId: 'u-sergey', leadName: 'Сергей' }],
  });
  qc.setQueryData(keys.rights, { members: [{ user: maria, perms: ['kb'] }, { user: { ...maria, id: 'u-elena', name: 'Елена', role: 'hrbp' }, perms: [] }] });
  qc.setQueryData(['demo-users'], { users: [anna, maria], password: 'buddy2026' });
  const tasks = [
    { id: 't1', title: 'Запросить доступ к Stash', stepId: 's1', done: true },
    { id: 't2', title: 'Сделать MR', done: false },
  ];
  const goal = { id: 'g1', internId: 'u-anna', title: 'Закрыть SEARCH-101', metric: 'MR выкачен на staging', due: '2026-11-24', status: 'in_progress', progress: 50, tasks, note: 'жду ревью', createdBy: 'u-maria', createdAt: '' };
  const stageGoal = { ...goal, id: 'plan-w1', title: 'Пройти этап «1 неделя»', metric: 'Выполнены все шаги этапа', auto: true, progress: 50, note: undefined,
    tasks: [{ id: 's1', title: 'Получи доступ к Stash', stepId: 's1', done: true }, { id: 's2', title: 'Настрой avito-cli', stepId: 's2', done: false }] };
  qc.setQueryData(keys.goals(), { goals: [stageGoal, goal, { ...goal, id: 'g2', title: 'Экватор', status: 'done', progress: 100, note: undefined }] });
  qc.setQueryData(keys.goals('u-anna'), { goals: [] });
  const internCard = (extra = {}) => ({
    user: anna, day: 4, percent: 33, done: 1, total: 3, overdue: 1, nextStep: 'Stash', openQuestions: 1, goalsDone: 1, goalsTotal: 2,
    mentorName: 'Олег Кузнецов', pendingApprovals: 1, waitingHours: 30, openRequests: 2, alerts: ['отстаёт от плана: просрочено 1', 'ждёт вашего решения: 1 (уже 1 дн)'], ...extra,
  });
  qc.setQueryData(keys.dashboard, {
    interns: [internCard(), internCard({ user: { ...anna, id: 'u-x', name: 'Иван' }, overdue: 0, pendingApprovals: 0, openRequests: 0, goalsTotal: 0, alerts: [], quizzesPassed: 1, quizzesFailed: 1 })],
    approvals: [{ kind: 'access', externalId: 'ACC-7', userId: 'u-anna', userName: 'Анна Смирнова', title: 'Kubernetes, администрирование', waitingHours: 30, details: { justification: 'нужно для отладки', exception: 'Стажёрам админ-доступ не выдаётся.' } }],
    reminders: [{ id: 'r1', ownerId: 'u-maria', text: 'Проверить SEARCH-101', at: '2026-11-14T06:00:00Z', sent: false, created: '' }],
    newcomers: 1,
  });
  qc.setQueryData(keys.internRequests('u-anna'), {
    requests: [
      { id: 'act1', kind: 'access', title: 'Доступ: Stash', status: 'approved', externalId: 'ACC-1', comment: 'ок', createdAt: '2026-11-10T10:00:00Z', awaitingMe: false },
      { id: 'act2', kind: 'absence', title: 'Отгул 13.11', status: 'submitted', externalId: 'ABS-1', createdAt: '2026-11-11T10:00:00Z', awaitingMe: true,
        approval: { kind: 'absence', externalId: 'ABS-1', userId: 'u-anna', userName: 'Анна Смирнова', title: 'Отгул на 13.11 (пт)', waitingHours: 5, details: { reason: 'учёба' } } },
    ],
    questions: [
      { id: 'q_1', text: 'Где тестовый стенд?', status: 'open', mentorName: 'Олег Кузнецов', createdAt: '2026-11-11T11:00:00Z' },
      { id: 'q_2', text: 'Где логи?', status: 'answered', answer: 'В Observer', answeredBy: 'Мария Волкова', createdAt: '2026-11-10T11:00:00Z' },
    ],
    canDelete: true,
    canAnswer: true,
  });
  qc.setQueryData(['quizRun', 'quiz-dayoff'], qc.getQueryData(keys.quiz('quiz-dayoff')));
  return qc;
}

function html(node: ReactNode, qc = seeded()) {
  return renderToStaticMarkup(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

test('chat renders history, sources, action card and pending trace', () => {
  useUi.getState().go('chat');
  const out = html(<Chat user={anna} />);
  assert.match(out, /Хочу отгул в пятницу/);
  assert.match(out, /<strong>всё в порядке<\/strong>/);
  assert.match(out, /Как оформить отгул — Отгул/);
  assert.match(out, /Отправить заявку/);
  assert.match(out, /Ищу в базе знаний/);
  assert.match(out, /офлайн-агент/);
  const empty = seeded();
  empty.setQueryData(keys.chat, { messages: [], actions: [] });
  assert.match(html(<Chat user={anna} />, empty), /Привет, Анна/);
  assert.match(html(<Chat user={maria} />, empty), /Zero Bug Policy/);
  const fresh = makeQueryClient();
  assert.match(html(<Chat user={anna} />, fresh), /Загружаю историю/);
});

test('bubble variants', () => {
  const qc = seeded();
  const err = { ...msgs[1]!, status: 'error' as const, mode: 'mentor', sources: [], actionId: undefined };
  const out = html(<Bubble m={err} onSuggest={() => undefined} isLast={false} />, qc);
  assert.match(out, /failed/);
  assert.match(out, /from-mentor/);
  assert.doesNotMatch(out, /Помогло/);
  const rated = html(<Bubble m={{ ...msgs[1]!, feedback: true }} onSuggest={() => undefined} isLast />, qc);
  assert.match(rated, /aria-pressed="true"/);
  assert.match(rated, /Статус моих заявок/);
  assert.match(html(<Bubble m={{ ...msgs[1]!, mode: 'proactive' }} onSuggest={() => undefined} isLast={false} />, qc), /Напоминание AI-buddy/);
});

test('action card states', () => {
  const states: Partial<Action>[] = [
    {},
    { status: 'submitted', externalId: 'ABS-1', args: { date_from: '2026-11-13', date_to: '2026-11-16' } },
    { status: 'approved', result: { comment: 'Хорошего отдыха' } },
    { status: 'failed', error: 'Слишком поздно' },
    { kind: 'access', title: 'Stash: репозиторий — search-api', args: { system: 'stash', level: 'edit' }, preview: { level: 'edit', project: 'search-api', approver: 'Олег Кузнецов', approver_role: 'владелец проекта', approval: 'owner' } },
    { kind: 'access', title: 'VPN', status: 'approved', preview: { approval: 'auto', level: 'user' }, result: { comment: 'ok' }, externalId: 'ACC-1' },
    { kind: 'mentor_question', title: 'Вопрос наставнику', args: { question: 'Где стенд?' }, preview: {} },
    { kind: 'mentor_question', title: 'Вопрос наставнику', status: 'approved', result: { mentor: 'Олег', answer: 'search-staging' } },
  ];
  const outs = states.map((s) => html(<ActionCard action={{ ...absence, ...s } as Action} />));
  assert.match(outs[0]!, /Останется отгулов/);
  assert.match(outs[1]!, /На согласовании/);
  assert.match(outs[1]!, /13 ноября — 16 ноября/);
  assert.match(outs[2]!, /Хорошего отдыха/);
  assert.match(outs[3]!, /Слишком поздно/);
  assert.match(outs[4]!, /Для какой задачи/);
  assert.match(outs[4]!, /Олег Кузнецов, владелец проекта/);
  assert.match(outs[4]!, /редактирование/);
  assert.match(outs[4]!, /search-api/);
  assert.match(outs[5]!, /Выдан/);
  assert.match(html(<ActionCard action={{ ...absence, kind: 'access', title: 'VPN', preview: { approval: 'auto', level: 'read' } } as Action} />), /Сразу после отправки/);
  assert.match(outs[6]!, /Отправить наставнику/);
  assert.match(outs[7]!, /search-staging/);
  assert.equal(editableFields({ ...absence, kind: 'access', preview: { approval: 'auto' } }).length, 0);
  assert.deepEqual(facts({ ...absence, preview: {}, args: {}, kind: 'access' })[0], ['Уровень', '']);
});

test('plan and today panel', () => {
  const out = html(<PlanBody plan={plan} />);
  assert.match(out, /День 4/);
  assert.match(out, /следующий/);
  assert.match(out, /делает наставник/);
  assert.match(out, /просрочен с 6 ноября/);
  assert.match(out, /Нужна помощь/);
  assert.match(out, /Спросить buddy/);
  assert.match(html(<PlanBody plan={plan} internId="u-anna" internName="Анна Смирнова" />), /План стажёра/);
  assert.match(html(<PlanView />), /1 неделя/);
  const asLead = seeded();
  asLead.setQueryData(keys.me, { user: maria, perms: ['approvals'] });
  const lead = html(<PlanView internId="u-anna" />, asLead);
  assert.match(lead, /Анна Смирнова/);
  assert.match(lead, /Можно добавить свою цель/);
  assert.match(lead, /\+ цель/);
  // HR BP видит цели, но не ставит и не убирает их
  const asHr = seeded();
  asHr.setQueryData(keys.me, { user: { ...maria, id: 'u-elena', role: 'hrbp' }, perms: [] });
  assert.doesNotMatch(html(<PlanView internId="u-anna" />, asHr), /\+ цель/);
  // руководитель убирает цель, поставленную вручную; цели этапов — нет
  asLead.setQueryData(keys.goals('u-anna'), seeded().getQueryData(keys.goals()));
  const leadGoals = html(<PlanView internId="u-anna" />, asLead);
  assert.equal((leadGoals.match(/aria-label="Убрать цель/g) ?? []).length, 2);
  const own = html(<PlanView />);
  assert.match(own, /Цели стажировки/);
  assert.match(own, /Критерий успеха: MR выкачен на staging/);
  assert.match(own, /1 из 3 выполнено/);
  // цели этапов — из плана, шкала заполняется по шагам
  assert.match(own, /Пройти этап «1 неделя»/);
  assert.match(own, /1 из 2 шагов/);
  assert.match(own, /Цели, поставленные вручную/);
  assert.match(own, /1 из 2 задач/);
  assert.match(own, /шаг плана/);
  assert.match(own, /50%/);
  assert.doesNotMatch(own, /type="range"/);
  assert.match(own, /жду ревью/);
  const noGoals = seeded();
  noGoals.setQueryData(keys.goals(), { goals: [] });
  assert.match(html(<PlanView />, noGoals), /появятся вместе с планом/);
  assert.match(html(<PlanView />, makeQueryClient()), /Загружаю план/);
  const today = html(<TodayPanel />);
  assert.match(today, /день стажировки/);
  assert.match(today, /просрочено 1/);
  assert.match(today, /Запроси доступ к Stash/);
  assert.match(html(<TodayPanel />, makeQueryClient()), /aria-busy="true"/);
});

test('mentor and lead screens', () => {
  assert.match(html(<Inbox />), /Где стенд\?/);
  assert.match(html(<Inbox />), /в базе знаний/);
  assert.match(html(<AnswerForm q={{ id: 'q', internId: 'u', mentorId: 'm', text: 'Вопрос?', status: 'open', createdAt: '' }} who="Анна" />), /Сохранить как инструкцию/);
  const noQ = seeded();
  noQ.setQueryData(keys.inbox, { questions: [] });
  assert.match(html(<Inbox />, noQ), /Новых вопросов нет/);
  assert.match(html(<Approvals />), /Отгул 13.11/);
  const row = html(<ApprovalRow a={{ kind: 'access', externalId: 'A', userId: 'u', userName: 'Анна', title: 'Доступ', waitingHours: 30, details: { justification: 'для задачи', exception: 'Не положено стажёрам' } }} />);
  assert.match(row, /для задачи/);
  assert.match(row, /вне правил/);
  assert.match(row, /Сделать исключение/);
  assert.match(row, /ждёт 1 дн/);
  assert.match(row, /Напомнить…/);
  const noA = seeded();
  noA.setQueryData(keys.approvals, { approvals: [] });
  noA.setQueryData(keys.inbox, { questions: [] });
  assert.match(html(<Approvals />, noA), /Нет заявок и вопросов/);
  // вкладка «Заявки и вопросы»: ждущие решения, вопросы стажёров с ответом на месте, все заявки
  const tab = seeded();
  tab.setQueryData(keys.inbox, {
    questions: [
      { id: 'q1', internId: 'u-anna', internName: 'Анна Смирнова', mentorId: 'u-oleg', text: 'Где стенд?', status: 'open', createdAt: '2026-11-13T10:00:00Z' },
      { id: 'q2', internId: 'u-anna', internName: 'Анна Смирнова', mentorId: 'u-oleg', text: 'Где логи?', status: 'answered', answer: 'В Observer', answeredBy: 'Мария Волкова', createdAt: '2026-11-12T10:00:00Z' },
    ],
  });
  tab.setQueryData(keys.allRequests, {
    requests: [
      { id: 'r1', kind: 'access', title: 'Доступ: Stash', status: 'approved', externalId: 'ACC-1', comment: 'ок', createdAt: '2026-11-10T10:00:00Z', awaitingMe: false, internId: 'u-anna', internName: 'Анна Смирнова' },
      { id: 'r2', kind: 'absence', title: 'Отгул 13.11', status: 'submitted', createdAt: '2026-11-11T10:00:00Z', awaitingMe: true, internId: 'u-anna', internName: 'Анна Смирнова' },
    ],
  });
  const page = html(<Approvals />, tab);
  assert.match(page, /Ждут вашего решения · 1/);
  assert.match(page, /Вопросы стажёров · 1 без ответа/);
  assert.match(page, /Где стенд\?/);
  assert.match(page, /Ответ стажёру/);
  assert.match(page, /Отвеченные · 1/);
  assert.match(page, /Все заявки стажёров/);
  assert.match(page, /Доступ: Stash/);
  assert.match(page, /согласовано/);
  useUi.getState().go('team');
  const team = html(<Team />);
  assert.match(team, /1 без ответа/);
  assert.match(team, /1 из 2/);
  assert.match(team, /нет целей/);
  assert.match(team, /не поставлены/);
  assert.match(team, /просрочка/);
  assert.match(team, /отстаёт от плана: просрочено 1/);
  assert.match(team, /Наставник<\/dt><dd>Олег Кузнецов/);
  assert.match(team, /1 ждут вас · 1 дн/);
  // заявки стажёра — внутри его строки, раскрыты сразу; отдельного списка заявок стажёров нет
  assert.doesNotMatch(team, /Ждут вашего решения/);
  assert.doesNotMatch(team, /Kubernetes, администрирование/);
  // строка не раскрывается сама — заявки по кнопке в колонке «Заявки»
  assert.doesNotMatch(team, /row-expand/);
  assert.match(team, /aria-expanded="false"/);
  const withOthers = seeded();
  const dash = withOthers.getQueryData<{ approvals: unknown[] }>(keys.dashboard)!;
  withOthers.setQueryData(keys.dashboard, {
    ...dash,
    approvals: [...dash.approvals, { kind: 'absence', externalId: 'ABS-9', userId: 'u-pavel', userName: 'Павел Егоров', title: 'Отпуск 01.12–05.12', waitingHours: 2, details: {} }],
  });
  const other = html(<Team />, withOthers);
  assert.match(other, /Заявки сотрудников/);
  assert.match(other, /Отпуск 01.12–05.12/);
  assert.match(team, /Проверить SEARCH-101/);
  assert.match(team, /новых сотрудников ждут назначения/);
  useUi.getState().go('team', { internId: 'u-anna' });
  const det = html(<Team />);
  assert.match(det, /Все стажёры/);
  assert.match(det, /Наставники:/);
  assert.match(det, /Заявки стажёра/);
  assert.match(det, /1 не согласовано/);
  assert.match(det, /ждёт вашего решения/);
  assert.match(det, /согласовано/);
  // решение прямо в карточке стажёра
  assert.match(det, /Отгул на 13.11 \(пт\)/);
  assert.match(det, />Согласовать</);
  assert.match(det, />Отказать</);
  useUi.getState().go('team');
  const noT = seeded();
  noT.setQueryData(keys.dashboard, { interns: [], approvals: [], reminders: [], newcomers: 0 });
  assert.match(html(<Team />, noT), /нет стажёров/);
  const ins = html(<InsightsBody d={insights} />);
  assert.match(ins, /2,3 ч/);
  assert.match(ins, /0,4 с/);
  assert.match(html(<InsightsBody d={{ ...insights, avgAnswerSeconds: 0.02 }} />), /меньше 0,1 с/);
  assert.match(html(<InsightsBody d={{ ...insights, avgAnswerSeconds: 1.5 }} />), /1,5 с/);
  assert.match(ins, /90%/);
  assert.match(html(<InsightsBody d={{ ...insights, feedbacks: 0, byDay: [] }} />), /нет оценок/);
  assert.match(html(<Insights />), /контрольной группой/);
});

test('requests, knowledge, login, shell and app', () => {
  assert.match(html(<Requests />), /Отгул на 13.11/);
  const noR = seeded();
  noR.setQueryData(keys.chat, { messages: [], actions: [] });
  assert.match(html(<Requests />, noR), /Заявок пока нет/);
  useUi.getState().go('knowledge');
  assert.match(html(<Knowledge />), /Выбери инструкцию/);
  useUi.getState().go('knowledge', { docId: 'process-day-off' });
  const kb = html(<Knowledge />);
  assert.match(kb, /не регламент Авито/);
  assert.match(kb, /<li>шаг<\/li>/);
  assert.match(html(<Login onDone={() => undefined} />), /Анна Смирнова/);
  const views: View[] = ['chat', 'plan', 'requests', 'knowledge', 'inbox', 'approvals', 'team', 'insights'];
  for (const v of views) {
    useUi.getState().go(v);
    assert.match(html(<Shell user={v === 'inbox' ? oleg : v === 'approvals' ? maria : anna} />), /buddy/);
  }
  useUi.getState().go('chat');
  assert.match(html(<Root />), /Анна Смирнова/);
  assert.match(html(<App client={seeded()} />), /Мой план/);
  assert.match(html(<Root />, makeQueryClient()), /загружается/);
});

function asLead() {
  const qc = seeded();
  qc.setQueryData(keys.me, { user: maria, perms: ['kb', 'approvals', 'programs'] });
  return qc;
}

test('knowledge: folders, search, editor', () => {
  const tree = buildTree(['Пустая', 'Процессы/Релизы'], [
    { id: 'a', title: 'Б', tags: [], folder: 'Процессы', custom: false, editable: false },
    { id: 'b', title: 'А', tags: [], folder: 'Процессы', custom: false, editable: false },
    { id: 'c', title: 'Корень', tags: [], folder: '', custom: false, editable: false },
  ]);
  assert.deepEqual(tree.children.map((c) => c.name), ['Процессы', 'Пустая']);
  assert.deepEqual(tree.children[0]!.docs.map((d) => d.title), ['А', 'Б']);
  assert.equal(tree.children[0]!.children[0]!.path, 'Процессы/Релизы');
  assert.equal(tree.docs[0]!.id, 'c');
  useUi.getState().go('knowledge');
  const list = html(<Knowledge />);
  assert.match(list, /Поиск по инструкциям/);
  assert.match(list, /Процессы/);
  assert.match(list, /Отсутствия/);
  assert.doesNotMatch(list, /Новая инструкция/);
  const lead = html(<Knowledge />, asLead());
  assert.match(lead, /Новая инструкция/);
  assert.match(lead, /удалить/); // пустую папку можно удалить
  useUi.getState().go('knowledge', { docId: 'doc_1' });
  const custom = html(<Knowledge />, asLead());
  assert.match(custom, /Инструкция команды, Мария/);
  assert.match(custom, /Редактировать/);
  assert.match(custom, /Процессы \/ Релизы/);
  assert.match(custom, /Тест «Релизы»/);
  useUi.getState().go('knowledge', { docId: 'process-day-off' });
  const builtin = html(<Knowledge />);
  assert.match(builtin, /Проверь себя/);
  assert.match(builtin, /сдан, 100%/);
  assert.match(builtin, /Пройти ещё раз/);
  const qc = seeded();
  const doc = qc.getQueryData(keys.doc('doc_1')) as Parameters<typeof DocView>[0]['doc'];
  const ed = html(<DocEditor doc={doc} folders={['Процессы/Релизы']} onDone={() => undefined} />);
  assert.match(ed, /Редактирование/);
  assert.match(ed, /По вторникам/);
  assert.match(ed, /Кому видна/);
  assert.match(ed, /value="релиз"/);
  assert.match(html(<DocEditor folders={[]} onDone={() => undefined} />), /Новая инструкция/);
  // инструкция, оформленная из ответа наставника, подписана; удаление — с подтверждением, без системного диалога
  const fromAnswer = html(<DocView doc={{ ...doc, source: 'answer', updatedBy: 'Олег Кузнецов' }} onEdit={() => undefined} />, qc);
  assert.match(fromAnswer, /Из ответа на вопрос, Олег Кузнецов/);
  assert.match(fromAnswer, /aria-label="Удалить инструкцию «/);
  assert.match(builtin, /id="kb-doc"/);
  assert.match(html(<FolderForm value="X" onChange={() => undefined} onDone={() => undefined} />), /не глубже 4 уровней/);
});

test('quizzes: list, runner, review, editor', () => {
  useUi.getState().go('quizzes');
  const list = html(<Quizzes />);
  assert.match(list, /Сдано 1 из 3/);
  assert.match(list, /Попробовать снова/);
  assert.match(list, /Начать/);
  assert.match(list, /лучший результат/);
  assert.doesNotMatch(list, /Новый тест/);
  assert.match(html(<Quizzes />, asLead()), /Новый тест/);
  assert.match(html(<Quizzes />, asLead()), /Изменить/);
  const run = html(<QuizRunner id="quiz-dayoff" onExit={() => undefined} />);
  assert.match(run, /Кто согласует\?/);
  assert.match(run, /Отвечено 0 из 1/);
  assert.match(run, /disabled=""[^>]*>Проверить ответы/);
  assert.match(html(<QuizRunner id="none" onExit={() => undefined} />, makeQueryClient()), /Загружаю тест/);
  const review = (passed: boolean) =>
    html(
      <QuizReview
        result={{
          attempt: { id: 'a', quizId: 'q', title: 'Отгулы', correct: passed ? 2 : 1, total: 2, percent: passed ? 100 : 50, passed, at: '' },
          review: [
            { text: 'Кто?', options: ['А', 'Б'], chosen: 1, correct: 1, right: true, explain: 'Потому что' },
            { text: 'Когда?', options: ['Сейчас', 'Потом'], chosen: 0, correct: 1, right: false },
          ],
          stepsDone: passed ? ['w1-quiz'] : [],
        }}
        docIds={['process-day-off']}
        onRetry={() => undefined}
        onExit={() => undefined}
      />,
    );
  assert.match(review(true), /Тест сдан/);
  assert.match(review(true), /Шаг плана отмечен выполненным/);
  assert.match(review(false), /Пока не сдан/);
  assert.match(review(false), /Правильный ответ: Потом/);
  assert.match(review(false), /Перечитать инструкцию/);
  const hidden = html(
    <QuizReview
      result={{
        attempt: { id: 'a', quizId: 'q', title: 'Отгулы', correct: 0, total: 1, percent: 0, passed: false, at: '' },
        review: [{ text: 'Когда?', options: ['Сейчас', 'Потом'], chosen: 0, correct: -1, right: false, docId: 'process-day-off', docTitle: 'Как оформить отгул' }],
        stepsDone: [],
      }}
      docIds={[]}
      onRetry={() => undefined}
      onExit={() => undefined}
    />,
  );
  assert.doesNotMatch(hidden, /Правильный ответ/);
  assert.match(hidden, /Перечитай:.*Как оформить отгул/);
  assert.match(run, /Прервать тест/);
  assert.match(run, /чат и инструкции закрыты/);
  const ed = html(<QuizEditor id="quiz_2" onDone={() => undefined} />);
  assert.match(ed, /Редактирование теста/);
  assert.match(ed, /value="Когда\?"/);
  assert.match(ed, /\+ вариант/);
  assert.match(html(<QuizEditor id={null} onDone={() => undefined} />), /Новый тест/);
  assert.match(html(<QuizEditor id="none" onDone={() => undefined} />, makeQueryClient()), /Загружаю тест/);
  assert.match(html(<QuizForm onDone={() => undefined} />), /Добавить вопрос/);
  useUi.getState().go('quizzes', { quizId: 'quiz-dayoff' });
  assert.match(html(<Quizzes />), /Кто согласует/);
  useUi.getState().go('quizzes');
});

test('templates, programs, rights', () => {
  const p = html(<Programs />, asLead());
  assert.match(p, /Стажёр-разработчик/);
  assert.match(p, /встроенный/);
  assert.match(p, /Аналитик поиска/);
  assert.match(p, /изменил\(а\) Олег/);
  assert.match(p, /тестов: 1/);
  assert.match(p, /Сделать копию/);
  assert.match(p, /Добавить шаблон/);
  const qc = seeded();
  const tpls = (qc.getQueryData(keys.templates) as { templates: Parameters<typeof TemplateEditor>[0]['initial'][] }).templates;
  const te = html(<TemplateEditor initial={tpls[1]!} onDone={() => undefined} />);
  assert.match(te, /Редактирование шаблона/);
  assert.match(te, /\+ шаг в «2 неделя»/);
  assert.match(te, /наставник/);
  const se = html(<StepEditor periods={tpls[0]!.periods} steps={tpls[0]!.steps} saving={false} saveLabel="Сохранить" onSave={() => undefined} onCancel={() => undefined} initialOpen={2} />);
  assert.match(se, /Закрывается тестом/);
  assert.match(se, /Кто отмечает/);
  assert.match(se, /3 шага/);
  const pp = html(<ProgramPanel internId="u-anna" internName="Анна" />, asLead());
  assert.match(pp, /настроена под стажёра/);
  assert.match(pp, /Настроить под стажёра/);
  assert.match(pp, /Назначить другой шаблон/);
  assert.doesNotMatch(html(<ProgramPanel internId="u-anna" internName="Анна" />), /Настроить под стажёра/);
  const qr = html(<QuizResultsPanel internId="u-anna" />);
  assert.match(qr, /лучший результат/);
  assert.match(qr, /2 попытки/);
  assert.match(qr, /Попытка 1 · 10 ноября/);
  assert.match(qr, /Ошибка<\/span>: Наставник/);
  assert.match(qr, /Правильный ответ: Руководитель/);
  assert.match(qr, /разбор по вопросам недоступен/);
  assert.equal(html(<QuizResultsPanel internId="u-x" />, makeQueryClient()), '');
  const empty = seeded();
  empty.setQueryData(keys.quizResults('u-x'), { attempts: [] });
  assert.match(html(<QuizResultsPanel internId="u-x" />, empty), /ещё не проходил/);
  const tq = html(<TeamQuizResults />);
  assert.match(tq, /Результаты стажёров/);
  assert.match(tq, /Удалённый/);
  assert.match(tq, /—/);
  assert.match(html(<AttemptAnswers a={attemptsFor()} />), /Верно/);
  useUi.getState().go('quizzes');
  assert.match(html(<Quizzes role="lead" />, asLead()), /Результаты стажёров/);
  assert.doesNotMatch(html(<Quizzes role="intern" />), /Результаты стажёров/);
  const noAtt = seeded();
  noAtt.setQueryData(keys.teamQuizResults, { interns: [{ intern: anna, attempts: [] }] });
  assert.match(html(<TeamQuizResults />, noAtt), /ещё не проходили/);
  assert.match(html(<TeamQuizResults />, makeQueryClient()), /Загружаю результаты/);
  const r = html(<Rights />, asLead());
  assert.match(r, /Инструкции и тесты/);
  assert.match(r, /Согласование заявок/);
  assert.match(r, /выдано/);
  assert.match(r, /Елена/);
  // на телефоне заголовков таблицы нет: название права подписано у каждого переключателя
  assert.match(r, /class="cell-label">Инструкции и тесты/);
  assert.match(html(<Rights />, makeQueryClient()), /Загружаю команду/);
  useUi.getState().go('team', { internId: 'u-anna' });
  const detail = html(<Team />, asLead());
  assert.match(detail, /Программа обучения/);
  assert.match(detail, /Результаты тестов/);
  useUi.getState().go('team');
});

test('nav by perms, today checkboxes, quiz button in chat', () => {
  assert.ok(navFor('hrbp', new Set(['approvals'])).some((n) => n.view === 'approvals'));
  assert.ok(!navFor('hrbp', new Set()).some((n) => n.view === 'approvals'));
  assert.ok(navFor('engineer', new Set(['kb'])).some((n) => n.view === 'quizzes'));
  assert.ok(navFor('lead', new Set()).some((n) => n.view === 'rights'));
  const qc = seeded();
  qc.setQueryData(keys.plan, {
    ...plan,
    periods: plan.periods.map((p) => ({ ...p, steps: [...p.steps, { ...p.steps[0]!, id: 'qz', owner: 'intern' as const, title: 'Тест недели', quiz: 'quiz-dayoff', status: 'todo' as const }] })),
  });
  const today = html(<TodayPanel />, qc);
  assert.match(today, /role="checkbox"/);
  assert.match(today, /Отметить выполненным/);
  assert.match(today, /Пройти тест: Тест недели/);
  assert.match(today, /Готово/);
  assert.match(today, /33%/);
  assert.match(html(<Bubble m={{ ...msgs[1]!, quizId: 'quiz-dayoff', actionId: undefined }} onSuggest={() => undefined} isLast />), /Пройти тест/);
  const qp = seeded();
  qp.setQueryData(keys.plan, { ...plan, next: { ...plan.next!, quiz: 'quiz-dayoff' } });
  assert.match(html(<TodayPanel />, qp), /Пройти тест/);
  const own = html(<PlanBody plan={{ ...plan, customized: true, periods: plan.periods.map((p) => ({ ...p, steps: p.steps.map((s) => ({ ...s, quiz: 'quiz-dayoff' })) })) }} />);
  assert.match(own, /Программа «Стажёр-разработчик», настроена индивидуально/);
  assert.match(own, /Пройти тест/);
  const locked = seeded();
  locked.setQueryData(keys.me, { user: anna, perms: [], activeQuiz: { quizId: 'quiz-dayoff', title: 'Отгулы', until: '2030-01-01T00:00:00Z' } });
  useUi.getState().go('quizzes', { quizId: 'quiz-dayoff' });
  const ls = html(<Shell user={anna} />, locked);
  assert.match(ls, /Идёт тест «Отгулы»/);
  assert.match(ls, /disabled=""[^>]*>Чат/);
  assert.match(ls, /закрыто/);
  const views: View[] = ['quizzes', 'programs', 'rights'];
  for (const v of views) {
    useUi.getState().go(v);
    assert.match(html(<Shell user={maria} />, asLead()), /buddy/);
  }
  useUi.getState().go('chat');
});

function attemptsFor() {
  return { id: 'x', quizId: 'q', title: 'Т', correct: 1, total: 1, percent: 100, passed: true, at: '', review: [{ text: 'В?', options: ['А'], chosen: 0, correct: 0, right: true }] };
}

test('registration and department transfer', () => {
  const reg = html(<Register onDone={() => undefined} onBack={() => undefined} />);
  assert.match(reg, /Регистрация/);
  assert.match(reg, /ФИО/);
  assert.match(reg, /Рабочая почта/);
  assert.match(reg, /Пароль/);
  assert.match(reg, /назначит руководитель/);
  assert.doesNotMatch(reg, /<select/);
  assert.doesNotMatch(reg, /Отдел и руководитель/);
  assert.match(html(<Login onDone={() => undefined} />), /Зарегистрироваться/);
  assert.match(html(<Waiting user={{ ...anna, role: 'newcomer', email: 'a@example.com' }} />), /Ждём назначения/);
  const dep = html(<Department />);
  assert.match(dep, /Сотрудники отдела/);
  assert.match(dep, /oleg@example.com/);
  assert.match(dep, /зарегистрирован/);
  assert.match(dep, /Доставка — Сергей/);
  assert.match(dep, /Перевести/);
  assert.match(dep, /Новые сотрудники/);
  assert.match(dep, /Пётр Сидоров/);
  assert.match(dep, /Принять в отдел/);
  assert.match(dep, /petr@example.com/);
  assert.match(html(<Department />, makeQueryClient()), /Загружаю отдел/);
  assert.match(html(<Rights />, asLead()), /Сотрудники отдела/);
});

test('admin: nav, service, all employees; reject with reason; requests inline', () => {
  const admin = { ...maria, id: 'u-admin', login: 'admin', name: 'Ирина Белова', role: 'admin' as const, team: '', teamName: '' };
  const nav = navFor('admin', new Set(['admin', 'approvals', 'kb', 'programs'])).map((n) => n.label);
  for (const l of ['Стажёры', 'Сотрудники и права', 'Сервис', 'Шаблоны обучения', 'Инструкции', 'Тесты']) assert.ok(nav.includes(l), l);
  assert.ok(nav.includes('Заявки и вопросы'));
  assert.equal(navFor('lead', new Set(['approvals'])).filter((n) => n.view === 'approvals').length, 1);
  // право «Согласование заявок» открывает вкладку сотруднику, у которого её не было
  assert.ok(navFor('hrbp', new Set(['approvals'])).some((n) => n.label === 'Заявки и вопросы'));
  assert.ok(!navFor('hrbp', new Set()).some((n) => n.view === 'approvals'));
  const qc = seeded();
  qc.setQueryData(keys.me, { user: admin, perms: ['admin', 'approvals', 'kb', 'programs'] });
  qc.setQueryData(keys.service, {
    status: { store: 'ok', kv: 'ok', corp: 'outage', llm: 'off' },
    mode: 'offline',
    flags: { absenceActions: true, accessActions: false, mentorEscalation: true, faqFromAnswers: true, llm: false, proactive: true },
    toggleable: ['absenceActions', 'accessActions', 'proactive'],
    roles: { intern: 2, lead: 2, newcomer: 1 },
    pending: 3,
    newcomers: 1,
    eventsTotal: 40,
    audit: [
      { id: 'e1', userId: 'u-admin', userName: 'Ирина Белова', type: 'flag_changed', props: { flag: 'accessActions', enabled: false }, at: '2026-11-13T19:00:00Z' },
      { id: 'e2', userId: 'u-maria', userName: 'Мария Волкова', type: 'approval_decided', props: { approved: false }, at: '2026-11-13T18:00:00Z' },
    ],
  });
  const svc = html(<Service />, qc);
  assert.match(svc, /База данных/);
  assert.match(svc, /офлайн-режим/);
  assert.match(svc, /outage/);
  assert.match(svc, /Заявки на доступы/);
  assert.match(svc, /Запустить ИИ-крон сейчас/);
  assert.match(svc, /3<\/span> заявок ждут решения/);
  assert.match(svc, /изменён флаг: Заявки на доступы — выкл\./);
  assert.match(svc, /решение по заявке: отказ/);
  assert.equal(eventText({ id: 'x', userId: 'u', userName: '', type: 'user_role_changed', props: { role: 'lead' }, at: '' }), 'смена роли: Руководитель');
  assert.equal(eventText({ id: 'x', userId: 'u', userName: '', type: 'unknown_event', props: null, at: '' }), 'unknown_event');
  const dep = html(
    <AdminDepartment
      d={{
        members: [anna, oleg, { ...maria, team: 'search', teamName: 'Поиск объявлений' }, admin],
        newcomers: [{ ...anna, id: 'u-new', name: 'Пётр Сидоров', role: 'newcomer', team: '', email: 'petr@example.com' }],
        mentors: [oleg],
        teams: [
          { id: 'search', name: 'Поиск объявлений', leadId: 'u-maria', leadName: 'Мария Волкова' },
          { id: 'platform', name: 'Платформа', leadId: '', leadName: '' },
        ],
      }}
    />,
    qc,
  );
  assert.match(dep, /Все сотрудники/);
  assert.match(dep, /Платформа \(без руководителя\)/);
  assert.match(dep, /HR BP/);
  assert.match(dep, /Руководитель<\/option>/);
  assert.match(dep, />Принять</);
  assert.match(dep, /Пароль…/);
  assert.doesNotMatch(dep, /Ирина Белова/);
  assert.match(html(<Department />, qc), /Все сотрудники/);
  assert.match(html(<Rights />, qc), /всех отделов/);
  const rf = html(<RejectForm kind="absence" reason="" onReason={() => undefined} pending={false} onSubmit={() => undefined} onCancel={() => undefined} />);
  assert.match(rf, /Причина отказа/);
  assert.match(rf, /В эти даты релиз/);
  assert.match(rf, /disabled=""[^>]*>Отказать/);
  assert.doesNotMatch(html(<RejectForm kind="access" reason="Хватит чтения" onReason={() => undefined} pending={false} onSubmit={() => undefined} onCancel={() => undefined} />), /disabled=""[^>]*>Отказать/);
  const inline = html(<InternRequests internId="u-anna" compact />);
  assert.match(inline, /Вопросы наставнику/);
  // наставник, руководитель и администратор отвечают прямо здесь
  assert.match(inline, /Ответ стажёру/);
  assert.match(inline, /Где тестовый стенд\?/);
  assert.match(inline, /ждёт ответа/);
  assert.match(inline, /Ответ \(Мария Волкова\): В Observer/);
  assert.equal((inline.match(/>Удалить</g) ?? []).length, 4);
  const ro = seeded();
  const cur = ro.getQueryData<Record<string, unknown>>(keys.internRequests('u-anna'))!;
  ro.setQueryData(keys.internRequests('u-anna'), { ...cur, canDelete: false });
  assert.doesNotMatch(html(<InternRequests internId="u-anna" />, ro), />Удалить</);
  assert.match(html(<DeleteButton what="заявку" pending={false} onConfirm={() => undefined} />), /aria-label="Удалить заявку"/);
  assert.match(inline, /intern-requests compact/);
  assert.match(inline, />Отказать</);
  assert.match(inline, /aria-expanded="true"/);
  // когда ничего не ждёт решения или ответа, заявки свёрнуты — видна только сводка
  const quiet = seeded();
  const base = quiet.getQueryData<{ requests: { awaitingMe?: boolean; approval?: unknown }[]; questions: { status: string }[] }>(keys.internRequests('u-anna'))!;
  quiet.setQueryData(keys.internRequests('u-anna'), {
    ...base,
    requests: base.requests.map((r) => ({ ...r, awaitingMe: false, approval: undefined })),
    questions: base.questions.map((x) => ({ ...x, status: 'answered' })),
  });
  const folded = html(<InternRequests internId="u-anna" />, quiet);
  assert.match(folded, /aria-expanded="false"/);
  assert.match(folded, /всего \d/);
  assert.doesNotMatch(folded, /req-list/);
});

test('mentors panel: several mentors, lead edits', () => {
  const qc = seeded();
  qc.setQueryData(keys.me, { user: maria, perms: ['approvals'] });
  const intern = { ...anna, mentorId: 'u-oleg', mentorIds: ['u-oleg', 'u-pavel'], leadId: 'u-maria' };
  const out = html(<MentorsPanel intern={intern} names={['Олег Кузнецов', 'Павел Егоров']} />, qc);
  assert.match(out, /Олег Кузнецов/);
  assert.match(out, /Павел Егоров/);
  assert.match(out, /основной/);
  assert.match(out, /Убрать наставника/);
  const ro = seeded();
  ro.setQueryData(keys.me, { user: oleg, perms: [] });
  const view = html(<MentorsPanel intern={intern} names={['Олег Кузнецов', 'Павел Егоров']} />, ro);
  assert.doesNotMatch(view, /Убрать наставника|Добавить наставника/);
  assert.ok(navFor('lead', new Set()).some((n) => n.label === 'Заявки и вопросы'));
  assert.ok(navFor('mentor', new Set()).some((n) => n.label === 'Заявки и вопросы'));
});

test('interns filter: search, state, team, mentor', () => {
  const qc = seeded();
  const cards = (qc.getQueryData(keys.dashboard) as { interns: InternCard[] }).interns;
  const f = { q: '', state: 'all', team: '', mentor: '' };
  assert.equal(filterInterns(cards, f).length, 2);
  assert.deepEqual(filterInterns(cards, { ...f, q: 'иван' }).map((i) => i.user.name), ['Иван']);
  assert.deepEqual(filterInterns(cards, { ...f, state: 'late' }).map((i) => i.user.id), ['u-anna']);
  assert.deepEqual(filterInterns(cards, { ...f, state: 'nogoals' }).map((i) => i.user.id), ['u-x']);
  assert.equal(filterInterns(cards, { ...f, mentor: 'Олег Кузнецов' }).length, 2);
  assert.equal(filterInterns(cards, { ...f, mentor: 'Никто' }).length, 0);
  assert.equal(filterInterns(cards, { ...f, team: 'delivery' }).length, 0);
  const bar = html(<InternFilters interns={cards} value={{ ...f, state: 'late' }} onChange={() => undefined} shown={1} />);
  assert.match(bar, /Отстают от плана \(1\)/);
  assert.match(bar, /Сбросить/);
  assert.match(bar, /1 из 2/);
  assert.match(html(<Team />), /Найти: имя, шаг, программа/);
});
