// Общие тестовые данные фронтенда.
import type { Action, Insights, Message, Plan, User } from './api/schemas';

export const anna: User = {
  id: 'u-anna', login: 'anna', name: 'Анна Смирнова', role: 'intern', team: 'search', teamName: 'Поиск объявлений',
  mentorId: 'u-oleg', leadId: 'u-maria', startDate: '2026-11-07T00:00:00+03:00',
};
export const maria: User = { ...anna, id: 'u-maria', login: 'maria', name: 'Мария Волкова', role: 'lead', mentorId: undefined, leadId: undefined };
export const oleg: User = { ...anna, id: 'u-oleg', login: 'oleg', name: 'Олег Кузнецов', role: 'mentor' };

export const absence: Action = {
  id: 'a1', userId: 'u-anna', kind: 'absence', tool: 'absence_create', title: 'Отгул на 13.11 (пт)',
  args: { type: 'day_off', date_from: '2026-11-13', date_to: '2026-11-13' },
  preview: { days: 1, approver: 'Мария Волкова', balance_after: 2 }, status: 'proposed', result: {},
  createdAt: '2026-11-10T12:00:00+03:00', updatedAt: '2026-11-10T12:00:00+03:00',
};

export const msgs: Message[] = [
  { id: 'm1', userId: 'u-anna', author: 'user', text: 'Хочу отгул в пятницу', status: 'done', sources: [], suggestions: [], trace: [], createdAt: '2026-11-10T12:00:00.000001+03:00' },
  {
    id: 'm2', userId: 'u-anna', author: 'assistant', text: 'Проверил по правилам — **всё в порядке**.\n- пункт', status: 'done', actionId: 'a1',
    sources: [{ docId: 'process-day-off', title: 'Как оформить отгул', section: 'Отгул' }], suggestions: ['Статус моих заявок'], trace: [], mode: 'offline',
    createdAt: '2026-11-10T12:00:00.000002+03:00',
  },
  { id: 'm3', userId: 'u-anna', author: 'assistant', text: '', status: 'pending', sources: [], suggestions: [], trace: [{ kind: 'tool', label: 'Ищу в базе знаний', at: '' }], createdAt: '2026-11-10T12:00:01+03:00' },
];

export const plan: Plan = {
  userId: 'u-anna', day: 4, currentPeriod: 'week1', done: 1, total: 3, percent: 33, overdue: 1,
  next: { id: 's2', period: 'week1', owner: 'intern', category: 'Доступы', title: 'Получи доступ к Stash', description: 'd', prompt: 'Запроси доступ к Stash', status: 'todo', due: '2026-11-13', overdue: false },
  periods: [
    { id: 'week0', name: 'Подготовка', state: 'past', done: 0, total: 1, steps: [
      { id: 's0', period: 'week0', owner: 'intern', category: 'Команда', title: 'Старый шаг', description: 'd', status: 'blocked', note: 'Нужна помощь', due: '2026-11-06', overdue: true },
    ] },
    { id: 'week1', name: '1 неделя', state: 'current', done: 1, total: 2, steps: [
      { id: 's1', period: 'week1', owner: 'mentor', category: 'Организационное', title: 'Назначить 1:1', description: 'd', status: 'done', due: '2026-11-13', overdue: false },
      { id: 's2', period: 'week1', owner: 'intern', category: 'Доступы', title: 'Получи доступ к Stash', description: 'd', prompt: 'Запроси доступ к Stash', status: 'todo', due: '2026-11-13', overdue: false },
    ] },
  ],
};

export const insights: Insights = {
  questions: 10, answeredByBuddy: 9, selfServeRate: 90, escalations: 1, mentorAnswers: 1, actionsSubmitted: 3, actionsCompleted: 2,
  helpfulRate: 80, feedbacks: 5, avgAnswerSeconds: 0.4, stepsCompleted: 4, mentorMinutesSaved: 135,
  byDay: [{ day: '2026-11-10', questions: 10, escalations: 1 }],
};
