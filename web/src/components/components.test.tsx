import assert from 'node:assert/strict';
import { test } from 'node:test';
import { QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { App, Root } from '../App';
import type { Action } from '../api/schemas';
import { keys, makeQueryClient } from '../hooks/queries';
import { uiStore, useUi, type View } from '../store/ui';
import { absence, anna, insights, maria, msgs, oleg, plan } from '../test-fixtures';
import { ActionCard, editableFields, facts } from './ActionCard';
import { ApprovalRow, Approvals } from './Approvals';
import { Bubble, Chat } from './Chat';
import { AnswerForm, Inbox } from './Inbox';
import { Insights, InsightsBody } from './Insights';
import { buildTree, DocEditor, DocView, FolderForm, Knowledge } from './Knowledge';
import { ProgramPanel, Programs, QuizResultsPanel, StepEditor, TemplateEditor } from './Programs';
import { QuizEditor, QuizForm, QuizReview, QuizRunner, Quizzes } from './Quizzes';
import { Rights } from './Rights';
import { Login } from './Login';
import { PlanBody, PlanView } from './PlanView';
import { Requests } from './Requests';
import { navFor, Shell } from './Shell';
import { Team } from './Team';
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
  qc.setQueryData(keys.quizResults('u-anna'), { attempts: [
    { id: 'a1', quizId: 'quiz-dayoff', title: 'Отгулы', correct: 1, total: 3, percent: 33, passed: false, at: '2026-11-10T12:00:00Z' },
    { id: 'a2', quizId: 'quiz-dayoff', title: 'Отгулы', correct: 3, total: 3, percent: 100, passed: true, at: '2026-11-10T13:00:00Z' },
  ] });
  qc.setQueryData(keys.rights, { members: [{ user: maria, perms: ['kb'] }, { user: { ...maria, id: 'u-elena', name: 'Елена', role: 'hrbp' }, perms: [] }] });
  qc.setQueryData(['demo-users'], { users: [anna, maria], password: 'buddy2026' });
  const goal = { id: 'g1', internId: 'u-anna', title: 'Закрыть SEARCH-101', metric: 'MR выкачен на staging', due: '2026-11-24', status: 'in_progress', progress: 40, note: 'жду ревью', createdBy: 'u-maria', createdAt: '' };
  qc.setQueryData(keys.goals(), { goals: [goal, { ...goal, id: 'g2', title: 'Экватор', status: 'done', progress: 100, note: undefined }] });
  qc.setQueryData(keys.goals('u-anna'), { goals: [] });
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
  const lead = html(<PlanView internId="u-anna" />);
  assert.match(lead, /Анна Смирнова/);
  assert.match(lead, /Поставьте стажёру/);
  assert.match(lead, /Добавить цель/);
  const own = html(<PlanView />);
  assert.match(own, /Цели стажировки/);
  assert.match(own, /Критерий успеха: MR выкачен на staging/);
  assert.match(own, /1 из 2 выполнено/);
  assert.match(own, /Вернуть в работу/);
  assert.match(own, /жду ревью/);
  const noGoals = seeded();
  noGoals.setQueryData(keys.goals(), { goals: [] });
  assert.match(html(<PlanView />, noGoals), /согласует руководитель/);
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
  assert.match(html(<AnswerForm q={{ id: 'q', internId: 'u', mentorId: 'm', text: 'Вопрос?', status: 'open', createdAt: '' }} who="Анна" />), /Сохранить в базу знаний/);
  const noQ = seeded();
  noQ.setQueryData(keys.inbox, { questions: [] });
  assert.match(html(<Inbox />, noQ), /Новых вопросов нет/);
  assert.match(html(<Approvals />), /Отгул 13.11/);
  assert.match(html(<ApprovalRow a={{ kind: 'access', externalId: 'A', userId: 'u', userName: 'Анна', title: 'Доступ', details: { justification: 'для задачи' } }} />), /для задачи/);
  const noA = seeded();
  noA.setQueryData(keys.approvals, { approvals: [] });
  assert.match(html(<Approvals />, noA), /Нечего согласовывать/);
  useUi.getState().go('team');
  const team = html(<Team />);
  assert.match(team, /1 без ответа/);
  assert.match(team, /1 из 2/);
  assert.match(team, /не поставлены/);
  useUi.getState().go('team', { internId: 'u-anna' });
  assert.match(html(<Team />), /Все стажёры/);
  useUi.getState().go('team');
  const noT = seeded();
  noT.setQueryData(keys.team, { interns: [] });
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
  assert.match(qr, /сдан/);
  assert.match(qr, /попыток: 2/);
  assert.equal(html(<QuizResultsPanel internId="u-x" />, makeQueryClient()), '');
  const r = html(<Rights />, asLead());
  assert.match(r, /Инструкции и тесты/);
  assert.match(r, /Согласование заявок/);
  assert.match(r, /выдано/);
  assert.match(r, /Елена/);
  assert.match(html(<Rights />, makeQueryClient()), /Загружаю команду/);
  useUi.getState().go('team', { internId: 'u-anna' });
  const detail = html(<Team />, asLead());
  assert.match(detail, /Программа обучения/);
  assert.match(detail, /Тесты/);
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
  const views: View[] = ['quizzes', 'programs', 'rights'];
  for (const v of views) {
    useUi.getState().go(v);
    assert.match(html(<Shell user={maria} />, asLead()), /buddy/);
  }
  useUi.getState().go('chat');
});
