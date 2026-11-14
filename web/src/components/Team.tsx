import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { InternCard, Reminder, User } from '../api/schemas';
import { keys, useMe } from '../hooks/queries';
import { dayMonth, initials, relative, remindAt, remindChoices, reqStatus, time, waited } from '../lib/format';
import { useUi } from '../store/ui';
import { ApprovalRow } from './Approvals';
import { DeleteButton } from './DeleteButton';
export { DeleteButton };
import { AnswerForm } from './Inbox';
import { PlanView } from './PlanView';
import { ProgramPanel } from './Programs';
import { QuizResultsPanel } from './QuizResults';

/** Кабинет: стажёры с отметками и сводкой при наведении, заявки на решение
 *  и напоминания — на одной странице, без лишних переходов. */
export function Team() {
  const { internId, go } = useUi();
  const [filter, setFilter] = useState<InternFilter>(loadFilter);
  const changeFilter = (f: InternFilter) => {
    setFilter(f);
    try {
      localStorage.setItem(filterKey, JSON.stringify(f));
    } catch {
      /* фильтр — только удобство: без хранилища просто не запоминается */
    }
  };
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
        {card && <MentorsPanel intern={card.user} names={card.mentorName ? card.mentorName.split(', ') : []} />}
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
  const shown = filterInterns(d.interns, filter);
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
          <>
            <InternFilters interns={d.interns} value={filter} onChange={changeFilter} shown={shown.length} />
            {shown.length === 0 && <p className="muted small">Никто не подходит под фильтр.</p>}
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
                {shown.map((i) => (
                  <InternRow key={i.user.id} i={i} onOpen={() => go('team', { internId: i.user.id })} />
                ))}
              </tbody>
            </table>
          </>
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
  const [open, setOpen] = useState(false);
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


/** Что стажёр просил согласовать: открытые заявки сверху, свои — с отметкой «ждёт вас». */
export function InternRequests({ internId, compact = false }: { internId: string; compact?: boolean }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const q = useQuery({ queryKey: keys.internRequests(internId), queryFn: () => api.internRequests(internId) });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: keys.internRequests(internId) });
    void qc.invalidateQueries({ queryKey: keys.dashboard });
    void qc.invalidateQueries({ queryKey: keys.approvals });
    void qc.invalidateQueries({ queryKey: keys.allRequests });
    void qc.invalidateQueries({ queryKey: keys.inbox });
  };
  const delReq = useMutation({
    mutationFn: (id: string) => api.deleteRequest(internId, id),
    onSuccess: () => {
      toast('Заявка удалена — стажёр получил уведомление');
      refresh();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const delQ = useMutation({
    mutationFn: (id: string) => api.deleteQuestion(internId, id),
    onSuccess: () => {
      toast('Вопрос удалён — у наставника он больше не висит');
      refresh();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  // свёрнуто, пока ничего не ждёт решения или ответа; раскрыть и свернуть можно в любой момент
  // (начальное состояние фиксируется при первой загрузке, чтобы после решения блок не схлопывался сам)
  const [expanded, setExpanded] = useState<boolean | null>(null);
  const initial = useRef<boolean | null>(null);
  if (q.isPending) return null;
  if (q.isError) return <p className="error small">Заявки не загрузились: {q.error.message}</p>;
  const { canDelete, canAnswer, questions } = q.data;
  const list = [...q.data.requests].sort((a, b) => Number(b.status === 'submitted') - Number(a.status === 'submitted'));
  const open = list.filter((r) => r.status === 'submitted').length;
  const openQ = questions.filter((x) => x.status === 'open').length;
  const needsMe = list.some((r) => r.awaitingMe) || (canAnswer && openQ > 0);
  if (initial.current === null) initial.current = needsMe;
  const show = expanded ?? initial.current;
  const bodyId = `ir-body-${internId}`;
  return (
    <section className={`intern-requests ${compact ? 'compact' : ''} ${show ? '' : 'collapsed'}`} aria-labelledby={`ir-title-${internId}`}>
      <button type="button" className="ir-head" aria-expanded={show} aria-controls={bodyId} onClick={() => setExpanded(!show)}>
        <span className={`chev ${show ? 'open' : ''}`} aria-hidden="true" />
        <h3 id={`ir-title-${internId}`}>Заявки стажёра</h3>
        <span className="small muted">
          {open > 0 ? `${open} не согласовано` : 'все решены'} · всего {list.length}
          {questions.length > 0 && ` · вопросов ${questions.length}${openQ ? `, без ответа ${openQ}` : ''}`}
        </span>
      </button>
      {show && (
        <div id={bodyId} className="ir-body">
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
                {canDelete && <DeleteButton what={`заявку «${r.title}»`} pending={delReq.isPending} onConfirm={() => delReq.mutate(r.id)} />}
              </li>
            ))}
          </ul>
          {questions.length > 0 && (
            <>
              <h3 className="ir-sub">Вопросы наставнику</h3>
              <ul className="req-list">
                {questions.map((x) => (
                  <li key={x.id} className={`req req-q-${x.status}`}>
                    <span className="req-title">«{x.text}»</span>
                    <span className={`stamp ${x.status === 'open' ? 'stamp-submitted' : 'stamp-approved'}`}>{x.status === 'open' ? 'ждёт ответа' : 'отвечен'}</span>
                    <span className="small muted">
                      {dayMonth(x.createdAt)}, {time(x.createdAt)}
                      {x.mentorName && ` · ${x.mentorName}`}
                    </span>
                    {x.answer && (
                      <span className="small">
                        Ответ{x.answeredBy && ` (${x.answeredBy})`}: {x.answer}
                      </span>
                    )}
                    {canAnswer && x.status === 'open' && (
                      <AnswerForm
                        q={{ id: x.id, internId, mentorId: '', text: x.text, status: 'open', createdAt: x.createdAt }}
                        who={x.mentorName ? `Наставники: ${x.mentorName}` : 'Вопрос стажёра'}
                        compact
                      />
                    )}
                    {canDelete && <DeleteButton what="вопрос" pending={delQ.isPending} onConfirm={() => delQ.mutate(x.id)} />}
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}
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
              <DeleteButton what={`напоминание «${r.text}»`} note="Удалить напоминание?" pending={del.isPending} onConfirm={() => del.mutate(r.id)} />
            </li>
          ))}
        </ul>
      )}
      {list.some((r) => r.sent) && <p className="small muted">Последнее сработало {relative(list.filter((r) => r.sent).at(-1)!.at)}</p>}
    </section>
  );
}

/** Наставники стажёра: руководитель стажёра и администратор назначают одного или нескольких. */
export function MentorsPanel({ intern, names = [] }: { intern: User; names?: string[] }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const me = useMe().data?.user;
  const canEdit = !!me && (me.role === 'admin' || (me.role === 'lead' && intern.leadId === me.id));
  const dep = useQuery({ queryKey: keys.department, queryFn: api.department, enabled: canEdit });
  const ids = [intern.mentorId, ...(intern.mentorIds ?? [])].filter((x, i, a): x is string => !!x && a.indexOf(x) === i);
  const people = new Map((dep.data?.mentors ?? []).map((m) => [m.id, m.name]));
  ids.forEach((id, i) => names[i] && !people.has(id) && people.set(id, names[i]));
  const [add, setAdd] = useState('');
  const m = useMutation({
    mutationFn: (next: string[]) => api.setMentors(intern.id, next),
    onSuccess: () => {
      setAdd('');
      toast('Наставники обновлены — стажёр и наставники получили уведомление');
      void qc.invalidateQueries({ queryKey: keys.dashboard });
      void qc.invalidateQueries({ queryKey: keys.team });
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const candidates = (dep.data?.mentors ?? []).filter((x) => !ids.includes(x.id));
  return (
    <section className="mentors-panel" aria-label="Наставники стажёра">
      <span className="small muted">Наставники:</span>
      <ul className="mentor-chips">
        {ids.map((id, i) => (
          <li key={id} className="mentor-chip">
            <span>{people.get(id) ?? id}</span>
            {i === 0 && ids.length > 1 && <span className="small muted"> · основной</span>}
            {canEdit && ids.length > 1 && (
              <button className="link small" aria-label={`Убрать наставника ${people.get(id) ?? id}`} disabled={m.isPending} onClick={() => m.mutate(ids.filter((x) => x !== id))}>
                ✕
              </button>
            )}
          </li>
        ))}
        {ids.length === 0 && <li className="small muted">не назначены</li>}
      </ul>
      {canEdit && candidates.length === 0 && dep.isSuccess && <span className="small muted">других наставников в отделе нет</span>}
      {canEdit && candidates.length > 0 && (
        <span className="mentor-add">
          <label className="sr-only" htmlFor={`add-mentor-${intern.id}`}>Добавить наставника</label>
          <select id={`add-mentor-${intern.id}`} value={add} onChange={(e) => setAdd(e.target.value)}>
            <option value="">Добавить наставника…</option>
            {candidates.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
                {me?.role === 'admin' && c.teamName ? ` — ${c.teamName}` : ''}
              </option>
            ))}
          </select>
          <button className="btn ghost small" disabled={!add || m.isPending} onClick={() => m.mutate([...ids, add])}>Добавить</button>
        </span>
      )}
    </section>
  );
}

/** Фильтр стажёров в кабинете: поиск, состояние, отдел, наставник. */
export type InternFilter = { q: string; state: string; team: string; mentor: string };
const emptyFilter: InternFilter = { q: '', state: 'all', team: '', mentor: '' };
const filterKey = 'aibuddy.internFilter';

function loadFilter(): InternFilter {
  try {
    const v = JSON.parse(localStorage.getItem(filterKey) ?? 'null') as InternFilter | null;
    return v && typeof v === 'object' ? { ...emptyFilter, ...v } : emptyFilter;
  } catch {
    return emptyFilter;
  }
}

export const internStates: { value: string; label: string; test: (i: InternCard) => boolean }[] = [
  { value: 'all', label: 'Все', test: () => true },
  { value: 'late', label: 'Отстают от плана', test: (i) => i.overdue > 0 },
  { value: 'waiting', label: 'Ждут моего решения', test: (i) => i.pendingApprovals > 0 },
  { value: 'questions', label: 'Есть вопросы без ответа', test: (i) => i.openQuestions > 0 },
  { value: 'quiz', label: 'Не сдан тест', test: (i) => i.quizzesFailed > 0 },
  { value: 'nogoals', label: 'Без своих целей', test: (i) => i.goalsTotal === 0 },
  { value: 'alerts', label: 'Требуют внимания', test: (i) => i.alerts.length > 0 },
];

/** filterInterns — стажёры, подходящие под фильтр. */
export function filterInterns(list: InternCard[], f: InternFilter): InternCard[] {
  const q = f.q.trim().toLowerCase();
  const state = internStates.find((s) => s.value === f.state) ?? internStates[0]!;
  return list.filter(
    (i) =>
      (!q || `${i.user.name} ${i.user.email ?? ''} ${i.nextStep ?? ''} ${i.templateName ?? ''}`.toLowerCase().includes(q)) &&
      state.test(i) &&
      (!f.team || i.user.team === f.team) &&
      (!f.mentor || i.mentorName.split(', ').includes(f.mentor)),
  );
}

export function InternFilters({ interns, value, onChange, shown }: { interns: InternCard[]; value: InternFilter; onChange: (f: InternFilter) => void; shown: number }) {
  const teams = [...new Map(interns.map((i) => [i.user.team, i.user.teamName ?? i.user.team])).entries()].filter(([id]) => id);
  const mentors = [...new Set(interns.flatMap((i) => (i.mentorName ? i.mentorName.split(', ') : [])))].sort();
  const set = (patch: Partial<InternFilter>) => onChange({ ...value, ...patch });
  const active = value.q || value.state !== 'all' || value.team || value.mentor;
  return (
    <div className="intern-filters" role="search" aria-label="Фильтр стажёров">
      <label className="sr-only" htmlFor="if-q">Найти стажёра</label>
      <input id="if-q" value={value.q} onChange={(e) => set({ q: e.target.value })} placeholder="Найти: имя, шаг, программа" />
      <label className="sr-only" htmlFor="if-state">Состояние</label>
      <select id="if-state" value={value.state} onChange={(e) => set({ state: e.target.value })}>
        {internStates.map((s) => (
          <option key={s.value} value={s.value}>
            {s.label} ({interns.filter(s.test).length})
          </option>
        ))}
      </select>
      {teams.length > 1 && (
        <>
          <label className="sr-only" htmlFor="if-team">Отдел</label>
          <select id="if-team" value={value.team} onChange={(e) => set({ team: e.target.value })}>
            <option value="">Все отделы</option>
            {teams.map(([id, name]) => (
              <option key={id} value={id}>{name}</option>
            ))}
          </select>
        </>
      )}
      {mentors.length > 1 && (
        <>
          <label className="sr-only" htmlFor="if-mentor">Наставник</label>
          <select id="if-mentor" value={value.mentor} onChange={(e) => set({ mentor: e.target.value })}>
            <option value="">Все наставники</option>
            {mentors.map((m) => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
        </>
      )}
      {active && (
        <>
          <span className="small muted">
            {shown} из {interns.length}
          </span>
          <button className="link small" onClick={() => onChange(emptyFilter)}>Сбросить</button>
        </>
      )}
    </div>
  );
}
