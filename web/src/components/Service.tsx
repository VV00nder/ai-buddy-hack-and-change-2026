import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { AuditEntry, ServiceInfo } from '../api/schemas';
import { keys } from '../hooks/queries';
import { dayMonth, roleName, time } from '../lib/format';
import { useUi } from '../store/ui';

const statusName: Record<string, string> = { store: 'База данных (PostgreSQL)', kv: 'Сессии и кэш (Redis)', corp: 'Корпоративные сервисы (MCP)', llm: 'Языковая модель' };

const flagName: Record<string, { title: string; hint: string }> = {
  absenceActions: { title: 'Заявки на отгул и отпуск', hint: 'AI-buddy оформляет черновики заявок на отсутствие' },
  accessActions: { title: 'Заявки на доступы', hint: 'AI-buddy оформляет заявки на доступ к системам' },
  mentorEscalation: { title: 'Вопрос наставнику', hint: 'Если ответа нет в инструкциях — вопрос уходит наставнику' },
  faqFromAnswers: { title: 'База ответов наставников', hint: 'Ответ наставника можно сохранить в инструкции команды' },
  proactive: { title: 'ИИ-крон: сводки и напоминания', hint: 'Утренние сводки стажёрам и напоминания о зависших заявках' },
};

const eventName: Record<string, string> = {
  login: 'вход',
  registered: 'регистрация',
  newcomer_assigned: 'принят(а) в отдел',
  user_role_changed: 'смена роли',
  employee_transferred: 'перевод в другой отдел',
  password_reset: 'сброс пароля',
  flag_changed: 'изменён флаг',
  cron_run: 'запуск ИИ-крона',
  approval_decided: 'решение по заявке',
  action_proposed: 'черновик заявки',
  action_submitted: 'заявка отправлена',
  action_completed: 'заявка выполнена',
  action_cancelled: 'заявка отменена',
  action_failed: 'ошибка заявки',
  right_changed: 'изменены права',
  question_answered: 'ответ AI-buddy',
  feedback: 'оценка ответа',
  mentor_escalation: 'вопрос наставнику',
  mentor_answered: 'ответ наставника',
  step_completed: 'шаг плана закрыт',
  quiz_started: 'начат тест',
  quiz_passed: 'тест сдан',
  quiz_failed: 'тест не сдан',
  quiz_saved: 'сохранён тест',
  kb_doc_saved: 'сохранена инструкция',
  template_saved: 'сохранён шаблон обучения',
  program_assigned: 'назначен шаблон',
  program_customized: 'программа настроена',
  reminder_sent: 'отправлено напоминание',
  reminder_set: 'поставлено напоминание',
  goal_created: 'поставлена цель',
  goal_done: 'цель выполнена',
};

/** Раздел администратора: состояние сервиса, флаги возможностей, ИИ-крон и журнал действий. */
export function Service() {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const q = useQuery({ queryKey: keys.service, queryFn: api.adminService, refetchInterval: 30_000 });
  const flag = useMutation({
    mutationFn: (v: { name: string; enabled: boolean }) => api.adminFlag(v.name, v.enabled),
    onSuccess: (_, v) => {
      toast(`${flagName[v.name]?.title ?? v.name}: ${v.enabled ? 'включено' : 'выключено'} для всех`);
      void qc.invalidateQueries({ queryKey: keys.service });
      void qc.invalidateQueries({ queryKey: keys.config });
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const cron = useMutation({
    mutationFn: api.adminCron,
    onSuccess: (r) => {
      toast(`ИИ-крон: сводок ${r.digests}, напоминаний о заявках ${r.staleRequests}, о вопросах ${r.openQuestions}, личных ${r.reminders}`);
      void qc.invalidateQueries({ queryKey: keys.service });
    },
    onError: (e) => toast(e.message, 'error'),
  });
  if (q.isPending) return <p className="muted">Загружаю состояние сервиса…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  const d = q.data;
  return (
    <div className="service">
      <section className="svc-block" aria-labelledby="svc-status">
        <h3 id="svc-status" className="section-title">Состояние</h3>
        <ul className="svc-status">
          {Object.entries(statusName).map(([k, name]) => {
            const v = d.status[k] ?? '—';
            const ok = v === 'ok' || (k === 'llm' && v === 'off');
            return (
              <li key={k} className={ok ? 'ok' : 'bad'}>
                <span className="dot" aria-hidden="true" />
                <span>{name}</span>
                <span className="small muted">{k === 'llm' ? llmText(d, v) : ok ? 'работает' : v}</span>
              </li>
            );
          })}
        </ul>
      </section>
      <section className="svc-block" aria-labelledby="svc-users">
        <h3 id="svc-users" className="section-title">Пользователи</h3>
        <ul className="cab-kpis">
          {Object.entries(d.roles)
            .sort((a, b) => b[1] - a[1])
            .map(([r, n]) => (
              <li key={r} className={r === 'newcomer' ? 'kpi warn' : 'kpi'}>
                <span className="num">{n}</span> {roleName[r as keyof typeof roleName] ?? r}
              </li>
            ))}
          <li className={d.pending ? 'kpi warn' : 'kpi'}>
            <span className="num">{d.pending}</span> заявок ждут решения
          </li>
        </ul>
        <p className="small muted">Роли, отделы и пароли — в разделе «Сотрудники и права».</p>
      </section>
      <section className="svc-block" aria-labelledby="svc-flags">
        <h3 id="svc-flags" className="section-title">Возможности</h3>
        <p className="small muted">Включаются и выключаются для всех сразу, без перезапуска. Режим языковой модели задаётся при запуске.</p>
        <ul className="svc-flags">
          {d.toggleable.map((name) => {
            const on = Boolean(d.flags[name as keyof typeof d.flags]);
            return (
              <li key={name}>
                <label className="switch">
                  <input
                    type="checkbox"
                    role="switch"
                    checked={on}
                    disabled={flag.isPending}
                    onChange={() => flag.mutate({ name, enabled: !on })}
                    aria-label={flagName[name]?.title ?? name}
                  />
                  <span>
                    {flagName[name]?.title ?? name}
                    <br />
                    <span className="small muted">{flagName[name]?.hint}</span>
                  </span>
                </label>
              </li>
            );
          })}
        </ul>
        <div className="row-start">
          <button className="btn small" disabled={cron.isPending} onClick={() => cron.mutate()}>
            Запустить ИИ-крон сейчас
          </button>
          <span className="small muted">Утренние сводки, напоминания о заявках и вопросах, личные напоминания</span>
        </div>
      </section>
      <section className="svc-block svc-audit" aria-labelledby="svc-audit">
        <h3 id="svc-audit" className="section-title">Журнал действий</h3>
        <p className="small muted">Последние {d.audit.length} из {d.eventsTotal} событий.</p>
        <table className="compact-table">
          <thead>
            <tr>
              <th scope="col">Когда</th>
              <th scope="col">Кто</th>
              <th scope="col">Событие</th>
            </tr>
          </thead>
          <tbody>
            {d.audit.map((e) => (
              <tr key={e.id}>
                <td className="small num">{dayMonth(e.at)}, {time(e.at)}</td>
                <td className="small">{e.userName || e.userId}</td>
                <td className="small">{eventText(e)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}

function llmText(d: ServiceInfo, v: string): string {
  if (v === 'off') return 'офлайн-режим (без внешней модели)';
  if (v === 'ok') return d.model ? `работает: ${d.model}` : 'работает';
  return v;
}

/** eventText — событие журнала по-русски с главными подробностями. */
export function eventText(e: AuditEntry): string {
  const p = (e.props ?? {}) as Record<string, unknown>;
  let t = eventName[e.type] ?? e.type;
  if (e.type === 'flag_changed') t += `: ${flagName[String(p.flag)]?.title ?? String(p.flag)} — ${p.enabled ? 'вкл.' : 'выкл.'}`;
  if (e.type === 'user_role_changed') t += `: ${roleName[p.role as keyof typeof roleName] ?? String(p.role)}`;
  if (e.type === 'approval_decided') t += p.approved ? ': согласовано' : ': отказ';
  return t;
}
