import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { InternCard, InternRequest, Reminder } from '../api/schemas';
import { keys } from '../hooks/queries';
import { dayMonth, initials, relative, remindAt, remindChoices, time, waited } from '../lib/format';
import { useUi } from '../store/ui';
import { ApprovalRow } from './Approvals';
import { PlanView } from './PlanView';
import { ProgramPanel } from './Programs';
import { QuizResultsPanel } from './QuizResults';

/** Кабинет: стажёры с отметками и сводкой при наведении, заявки на решение
 *  и напоминания — на одной странице, без лишних переходов. */
export function Team() {
  const { internId, go } = useUi();
  const q = useQuery({ queryKey: keys.dashboard, queryFn: api.dashboard });
  if (internId) {
    const card = q.data?.interns.find((i) => i.user.id === internId);
    const name = card?.user.name ?? 'стажёра';
    return (
      <div className="intern-detail">
        <button className="link back" onClick={() => go('team')}>Все стажёры</button>
        {card && card.alerts.length > 0 && (
          <ul className="alert-list" aria-label="Требует внимания">
            {card.alerts.map((a) => (
              <li key={a} className="alert-chip">{a}</li>
            ))}
          </ul>
        )}
        <InternRequests internId={internId} />
        <ProgramPanel internId={internId} internName={name} />
        <QuizResultsPanel internId={internId} />
        <PlanView internId={internId} />
      </div>
    );
  }
  if (q.isPending) return <p className="muted">Загружаю кабинет…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  const d = q.data;
  const behind = d.interns.filter((i) => i.overdue > 0).length;
  // заявки стажёров — внутри строки стажёра; здесь только заявки остальных сотрудников (если есть)
  const internIds = new Set(d.interns.map((i) => i.user.id));
  const others = d.approvals.filter((a) => !internIds.has(a.userId));
  return (
    <div className="cabinet">
      <ul className="cab-kpis" aria-label="Сводка">
        <li className={d.approvals.length ? 'kpi warn' : 'kpi'}>
          <span className="num">{d.approvals.length}</span> ждут вашего решения
        </li>
        <li className={behind ? 'kpi warn' : 'kpi'}>
          <span className="num">{behind}</span> отстают от плана
        </li>
        <li className="kpi">
          <span className="num">{d.interns.length}</span> стажёров
        </li>
        {d.newcomers > 0 && (
          <li className="kpi warn">
            <button className="link" onClick={() => go('rights')}>
              <span className="num">{d.newcomers}</span> новых сотрудников ждут назначения
            </button>
          </li>
        )}
      </ul>
      <section className="cab-interns" aria-label="Стажёры">
        {d.interns.length === 0 ? (
          <div className="empty">
            <h2>У вас нет стажёров</h2>
          </div>
        ) : (
          <table className="compact-table">
            <thead>
              <tr>
                <th scope="col">Стажёр</th>
                <th scope="col">День</th>
                <th scope="col">План</th>
                <th scope="col">Цели</th>
                <th scope="col">Тесты</th>
                <th scope="col">Заявки</th>
                <th scope="col">Следующий шаг</th>
              </tr>
            </thead>
            <tbody>
              {d.interns.map((i) => (
                <InternRow key={i.user.id} i={i} onOpen={() => go('team', { internId: i.user.id })} />
              ))}
            </tbody>
          </table>
        )}
      </section>
      <aside className="cab-side">
        {others.length > 0 && (
          <section aria-labelledby="cab-approvals">
            <h3 id="cab-approvals" className="section-title">Заявки сотрудников</h3>
            {others.map((a) => (
              <ApprovalRow key={a.externalId} a={a} compact />
            ))}
          </section>
        )}
        <Reminders list={d.reminders} />
      </aside>
    </div>
  );
}

function InternRow({ i, onOpen }: { i: InternCard; onOpen: () => void }) {
  const late = i.overdue > 0;
  // заявки стажёра раскрываются прямо в таблице — решение без перехода в карточку
  // заявки, ждущие решения, раскрыты сразу — согласовать или отказать можно без лишнего клика
  const [open, setOpen] = useState(i.pendingApprovals > 0);
  useEffect(() => {
    if (i.pendingApprovals > 0) setOpen(true); // новая заявка пришла, пока страница открыта
  }, [i.pendingApprovals]);
  return (
    <>
      <tr className={`${late ? 'row-late' : ''} ${open ? 'row-open' : ''}`} onClick={onOpen}>
        <td>
          <div className="person-wrap">
            <button className="person" onClick={onOpen} aria-describedby={`sum-${i.user.id}`}>
              <span className="avatar role-intern" aria-hidden="true">{initials(i.user.name)}</span>
              <span>
                {i.user.name}
                {late && <span className="late-mark" title={`Просрочено шагов: ${i.overdue}`}> просрочка</span>}
                <br />
                <span className="muted small">{i.templateName || i.user.teamName}</span>
              </span>
            </button>
            <InternSummaryPop i={i} />
          </div>
          {i.alerts.length > 0 && (
            <ul className="row-alerts">
              {i.alerts.map((a) => (
                <li key={a}>{a}</li>
              ))}
            </ul>
          )}
        </td>
        <td className="num">{i.day}</td>
        <td>
          <div className="progress-line">
            <div className="progress" aria-label={`${i.percent}%`}><span style={{ width: `${i.percent}%` }} /></div>
            <span className="small num pct">{i.percent}%</span>
          </div>
          <span className="small">
            {i.done}/{i.total}
            {late && <span className="overdue-text">, просрочено {i.overdue}</span>}
          </span>
        </td>
        <td className="small">{i.goalsTotal > 0 ? `${i.goalsDone} из ${i.goalsTotal}` : <span className="overdue-text">нет целей</span>}</td>
        <td className="small">
          {i.quizzesPassed + i.quizzesFailed === 0 ? (
            <span className="muted">—</span>
          ) : (
            <>
              {i.quizzesPassed} сдано
              {i.quizzesFailed > 0 && <span className="overdue-text">, {i.quizzesFailed} нет</span>}
            </>
          )}
        </td>
        <td className="small">
          <button
            className={`req-toggle ${i.pendingApprovals > 0 ? (i.waitingHours >= 24 ? 'tag overdue-tag' : 'tag') : 'link small'}`}
            aria-expanded={open}
            aria-controls={`req-${i.user.id}`}
            onClick={(e) => {
              e.stopPropagation();
              setOpen(!open);
            }}
          >
            {i.pendingApprovals > 0
              ? `${i.pendingApprovals} ждут вас · ${waited(i.waitingHours)}`
              : i.openRequests > 0
                ? `${i.openRequests} на согласовании`
                : 'история'}
            <span aria-hidden="true"> {open ? '▴' : '▾'}</span>
          </button>
        </td>
        <td className="small">{i.nextStep ?? 'Всё выполнено'}</td>
      </tr>
      {open && (
        <tr className="row-expand" id={`req-${i.user.id}`}>
          <td colSpan={7}>
            <InternRequests internId={i.user.id} compact />
          </td>
        </tr>
      )}
    </>
  );
}

/** Краткая сводка о стажёре — появляется при наведении или фокусе на имени. */
export function InternSummaryPop({ i }: { i: InternCard }) {
  return (
    <div className="summary-pop" role="tooltip" id={`sum-${i.user.id}`}>
      <strong>{i.user.name}</strong>
      <dl>
        <dt>День стажировки</dt>
        <dd>{i.day}-й</dd>
        <dt>План</dt>
        <dd>
          {i.percent}% · {i.done} из {i.total}
          {i.overdue > 0 && <span className="overdue-text">, просрочено {i.overdue}</span>}
        </dd>
        <dt>Следующий шаг</dt>
        <dd>{i.nextStep ?? 'всё выполнено'}</dd>
        <dt>Наставник</dt>
        <dd>{i.mentorName || '—'}</dd>
        <dt>Цели</dt>
        <dd>{i.goalsTotal > 0 ? `${i.goalsDone} из ${i.goalsTotal} выполнено` : 'не поставлены'}</dd>
        <dt>Тесты</dt>
        <dd>{i.quizzesPassed + i.quizzesFailed > 0 ? `сдано ${i.quizzesPassed}, не сдано ${i.quizzesFailed}` : 'ещё не проходил(а)'}</dd>
        <dt>Вопросы</dt>
        <dd>{i.openQuestions > 0 ? `${i.openQuestions} без ответа` : 'нет открытых'}</dd>
        <dt>Заявки</dt>
        <dd>{i.pendingApprovals > 0 ? `${i.pendingApprovals} ждут вашего решения` : i.openRequests > 0 ? `${i.openRequests} на согласовании` : 'нет открытых'}</dd>
      </dl>
    </div>
  );
}

const reqStatus: Record<InternRequest['status'], string> = {
  proposed: 'черновик',
  submitted: 'ждёт решения',
  approved: 'согласовано',
  rejected: 'отклонено',
  cancelled: 'отменено',
  failed: 'ошибка',
};

/** Что стажёр просил согласовать: открытые заявки сверху, свои — с отметкой «ждёт вас». */
export function InternRequests({ internId, compact = false }: { internId: string; compact?: boolean }) {
  const q = useQuery({ queryKey: keys.internRequests(internId), queryFn: () => api.internRequests(internId) });
  if (q.isPending) return null;
  if (q.isError) return <p className="error small">Заявки не загрузились: {q.error.message}</p>;
  const list = [...q.data.requests].sort((a, b) => Number(b.status === 'submitted') - Number(a.status === 'submitted'));
  const open = list.filter((r) => r.status === 'submitted').length;
  return (
    <section className={`intern-requests ${compact ? 'compact' : ''}`} aria-labelledby={`ir-title-${internId}`}>
      <div className="row">
        <h3 id={`ir-title-${internId}`}>Заявки стажёра</h3>
        <span className="small muted">
          {open > 0 ? `${open} не согласовано` : 'все решены'} · всего {list.length}
        </span>
      </div>
      {list.length === 0 && <p className="small muted">Стажёр ещё не отправлял заявок.</p>}
      <ul className="req-list">
        {list.map((r) => (
          <li key={r.id} className={`req req-${r.status} ${r.awaitingMe ? 'req-mine' : ''}`}>
            {r.approval ? (
              // заявка ждёт решения текущего пользователя — согласовать или отказать прямо здесь
              <ApprovalRow a={r.approval} compact />
            ) : (
              <>
                <span className="req-title">{r.title}</span>
                <span className={`stamp stamp-${r.status}`}>{r.awaitingMe ? 'ждёт вашего решения' : reqStatus[r.status]}</span>
                <span className="small muted">
                  {dayMonth(r.createdAt)}, {time(r.createdAt)}
                  {r.externalId && ` · ${r.externalId}`}
                </span>
                {r.comment && <span className="small">«{r.comment}»</span>}
              </>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Свои напоминания: на заявку, задачу или что угодно — придут уведомлением. */
export function Reminders({ list }: { list: Reminder[] }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [text, setText] = useState('');
  const [when, setWhen] = useState('3');
  const refresh = () => void qc.invalidateQueries({ queryKey: keys.dashboard });
  const add = useMutation({
    mutationFn: () => api.addReminder({ text, ...remindAt(when) }),
    onSuccess: () => {
      setText('');
      toast('Напоминание поставлено');
      refresh();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const del = useMutation({ mutationFn: api.deleteReminder, onSuccess: refresh, onError: (e) => toast(e.message, 'error') });
  function submit(e: FormEvent) {
    e.preventDefault();
    add.mutate();
  }
  const upcoming = list.filter((r) => !r.sent);
  return (
    <section className="reminders" aria-labelledby="rem-title">
      <h3 id="rem-title" className="section-title">Напоминания</h3>
      <form className="rem-form" onSubmit={submit}>
        <label className="sr-only" htmlFor="rem-text">Текст напоминания</label>
        <input id="rem-text" value={text} onChange={(e) => setText(e.target.value)} placeholder="Например: проверить задачу SEARCH-101" minLength={3} maxLength={300} required />
        <label className="sr-only" htmlFor="rem-when">Когда</label>
        <select id="rem-when" value={when} onChange={(e) => setWhen(e.target.value)}>
          {remindChoices.map((c) => (
            <option key={c.value} value={c.value}>{c.label}</option>
          ))}
        </select>
        <button className="btn small" disabled={add.isPending}>Напомнить</button>
      </form>
      {upcoming.length === 0 ? (
        <p className="small muted">Нет запланированных. Заявку можно отложить кнопкой «Напомнить…» — о висящих больше суток напомним сами.</p>
      ) : (
        <ul className="rem-list">
          {upcoming.map((r) => (
            <li key={r.id}>
              <span>{r.text}</span>
              <span className="small muted">{dayMonth(r.at)}, {time(r.at)}</span>
              <button className="link small" aria-label={`Удалить напоминание «${r.text}»`} disabled={del.isPending} onClick={() => del.mutate(r.id)}>✕</button>
            </li>
          ))}
        </ul>
      )}
      {list.some((r) => r.sent) && <p className="small muted">Последнее сработало {relative(list.filter((r) => r.sent).at(-1)!.at)}</p>}
    </section>
  );
}
