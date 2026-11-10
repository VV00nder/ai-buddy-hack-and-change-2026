// Форматирование дат, статусов и подписей интерфейса.
import type { Action, ActionStatus, Role, StepStatus } from '../api/schemas';

const months = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];

export function dayMonth(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  if (!m) return iso;
  return `${Number(m[3])} ${months[Number(m[2]) - 1]}`;
}

export function time(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
}

export function relative(iso: string, now: Date = new Date()): string {
  const d = new Date(iso);
  const sec = Math.round((now.getTime() - d.getTime()) / 1000);
  if (Number.isNaN(sec)) return '';
  if (sec < 60) return 'только что';
  if (sec < 3600) return `${Math.floor(sec / 60)} мин назад`;
  if (sec < 86400) return `${Math.floor(sec / 3600)} ч назад`;
  return dayMonth(iso);
}

export const roleName: Record<Role, string> = {
  intern: 'Стажёр',
  mentor: 'Наставник',
  lead: 'Руководитель',
  hrbp: 'HR BP',
};

export const stepStatusName: Record<StepStatus, string> = {
  todo: 'Не начат',
  in_progress: 'В работе',
  done: 'Готово',
  blocked: 'Есть блокер',
};

export const actionStatusName: Record<ActionStatus, string> = {
  proposed: 'Черновик',
  submitted: 'На согласовании',
  approved: 'Готово',
  rejected: 'Отклонено',
  cancelled: 'Отменено',
  failed: 'Не выполнено',
};

/** Надпись на «штампе» карточки — зависит от типа действия. */
export function stampText(a: Pick<Action, 'kind' | 'status'>): string {
  if (a.status === 'approved') {
    if (a.kind === 'access') return 'Выдан';
    if (a.kind === 'mentor_question') return 'Есть ответ';
    return 'Согласовано';
  }
  if (a.status === 'submitted' && a.kind === 'mentor_question') return 'У наставника';
  return actionStatusName[a.status];
}

export const kindName: Record<Action['kind'], string> = {
  absence: 'Заявка на отсутствие',
  access: 'Заявка на доступ',
  mentor_question: 'Вопрос наставнику',
};

export function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

export function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join('');
}
