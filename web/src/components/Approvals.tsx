import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { Approval } from '../api/schemas';
import { keys } from '../hooks/queries';
import { dayMonth, remindAt, remindChoices, reqStatus, time, waited } from '../lib/format';
import { AnswerForm } from './Inbox';
import { useUi } from '../store/ui';

/** Вкладка «Заявки и вопросы»: что ждёт решения, вопросы стажёров и все их заявки.
 *  Есть у руководителя, администратора, наставника и у сотрудника с правом «Согласование заявок». */
export function Approvals() {
  const q = useQuery({ queryKey: keys.approvals, queryFn: api.approvals });
  // вопросы и история видны не всем, у кого есть согласования (например, DevOps): 403 — раздел скрыт
  const inbox = useQuery({ queryKey: keys.inbox, queryFn: api.inbox, retry: false });
  const all = useQuery({ queryKey: keys.allRequests, queryFn: api.allRequests, retry: false });
  if (q.isPending) return <p className="muted">Загружаю заявки…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  const list = q.data.approvals;
  const questions = inbox.data?.questions ?? [];
  const open = questions.filter((x) => x.status === 'open');
  const answered = questions.filter((x) => x.status === 'answered');
  const history = (all.data?.requests ?? []).filter((r) => !r.awaitingMe);
  const nothing = list.length === 0 && questions.length === 0 && history.length === 0;
  return (
    <section className="approvals approvals-page">
      {nothing && (
        <div className="empty">
          <h2>Нет заявок и вопросов</h2>
          <p className="muted">Когда стажёр отправит заявку на отгул или доступ или задаст вопрос, они появятся здесь и в уведомлениях.</p>
        </div>
      )}
      {list.length === 0 && !nothing && <p className="muted small">Сейчас ничего не ждёт вашего решения.</p>}
      {list.length > 0 && (
        <section aria-labelledby="ap-wait">
          <h2 id="ap-wait" className="section-title">Ждут вашего решения · {list.length}</h2>
          <div className="ap-list">
            {list.map((a) => (
              <ApprovalRow key={a.externalId} a={a} />
            ))}
          </div>
        </section>
      )}
      {questions.length > 0 && (
        <section aria-labelledby="ap-q">
          <h2 id="ap-q" className="section-title">Вопросы стажёров{open.length > 0 && ` · ${open.length} без ответа`}</h2>
          {open.map((x) => (
            <AnswerForm key={x.id} q={x} who={x.internName ?? 'Стажёр'} />
          ))}
          {answered.length > 0 && (
            <details className="ap-answered">
              <summary className="small">Отвеченные · {answered.length}</summary>
              <ul className="answered">
                {answered.map((x) => (
                  <li key={x.id}>
                    <p>
                      <strong>{x.internName ?? 'Стажёр'}:</strong> {x.text}
                    </p>
                    <p className="muted">{x.answer}</p>
                    {x.answeredBy && <p className="small muted">Ответил(а): {x.answeredBy}</p>}
                  </li>
                ))}
              </ul>
            </details>
          )}
        </section>
      )}
      {history.length > 0 && (
        <section aria-labelledby="ap-all">
          <h2 id="ap-all" className="section-title">Все заявки стажёров</h2>
          <table className="compact-table">
            <thead>
              <tr>
                <th scope="col">Стажёр</th>
                <th scope="col">Заявка</th>
                <th scope="col">Статус</th>
                <th scope="col">Когда</th>
                <th scope="col">Комментарий</th>
              </tr>
            </thead>
            <tbody>
              {history.map((r) => (
                <tr key={r.id}>
                  <td className="small">{r.internName}</td>
                  <td className="small">
                    {r.title}
                    {r.externalId && <span className="muted"> · {r.externalId}</span>}
                  </td>
                  <td>
                    <span className={`stamp stamp-${r.status}`}>{reqStatus[r.status]}</span>
                  </td>
                  <td className="small num">
                    {dayMonth(r.createdAt)}, {time(r.createdAt)}
                  </td>
                  <td className="small">{r.comment ?? ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </section>
  );
}

/** Заявка на решение: согласовать или отклонить в один клик, комментарий — по желанию,
 *  «Напомнить» — если решение нужно отложить. */
export function ApprovalRow({ a, compact = false }: { a: Approval; compact?: boolean }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [comment, setComment] = useState('');
  const [withComment, setWithComment] = useState(!compact);
  // отказ — всегда с причиной: стажёр должен понимать, что делать дальше
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState('');
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: keys.approvals });
    void qc.invalidateQueries({ queryKey: keys.dashboard });
    void qc.invalidateQueries({ queryKey: ['internRequests'] });
    void qc.invalidateQueries({ queryKey: keys.allRequests });
  };
  const m = useMutation({
    mutationFn: (approve: boolean) => api.decide(a.kind, a.externalId, approve, approve ? comment : reason.trim()),
    onSuccess: (_, approve) => {
      toast(approve ? 'Согласовано — стажёр получил уведомление' : 'Отказано — стажёр получил уведомление с причиной');
      refresh();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const remind = useMutation({
    mutationFn: (choice: string) => api.addReminder({ text: `Решить заявку: ${a.userName} — ${a.title}`, link: '/approvals', ...remindAt(choice) }),
    onSuccess: () => {
      toast('Напомню о заявке');
      void qc.invalidateQueries({ queryKey: keys.dashboard });
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const d = a.details as Record<string, string | undefined>;
  const created = Date.parse(String(d.createdAt ?? ''));
  const hours = a.waitingHours || (Number.isNaN(created) ? 0 : Math.max(0, Math.floor((Date.now() - created) / 3_600_000)));
  const stale = hours >= 24;
  return (
    <article className={`note note-${a.kind} approval ${compact ? 'compact' : ''} ${stale ? 'stale' : ''}`}>
      <header>
        <span className="note-kind">{a.userName}</span>
        {d.exception && <span className="tag exception-tag" title={d.exception}>вне правил</span>}
        <span className={`small ${stale ? 'overdue-text' : 'muted'}`} title="Сколько заявка ждёт решения">
          ждёт {waited(hours)}
        </span>
      </header>
      <h3>{a.title}</h3>
      {d.exception && <p className="small exception-text">По правилам не положено: {d.exception}</p>}
      {(d.reason || d.justification) && <p className="small">«{d.reason ?? d.justification}»</p>}
      {rejecting ? (
        <RejectForm
          kind={a.kind}
          reason={reason}
          onReason={setReason}
          pending={m.isPending}
          onSubmit={() => m.mutate(false)}
          onCancel={() => setRejecting(false)}
        />
      ) : (
        <>
          {withComment ? (
            <label className="field">
              <span className="small">Комментарий стажёру</span>
              <input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Необязательно" />
            </label>
          ) : null}
          <div className="note-actions">
            <button className="btn small" disabled={m.isPending} onClick={() => m.mutate(true)}>{d.exception ? 'Сделать исключение' : 'Согласовать'}</button>
            <button className="btn ghost small" disabled={m.isPending} onClick={() => setRejecting(true)}>Отказать</button>
            {!withComment && (
              <button className="link small" onClick={() => setWithComment(true)}>+ комментарий</button>
            )}
            <label className="remind">
              <span className="sr-only">Напомнить о заявке {a.externalId}</span>
              <select value="" disabled={remind.isPending} onChange={(e) => e.target.value && remind.mutate(e.target.value)}>
                <option value="">Напомнить…</option>
                {remindChoices.map((c) => (
                  <option key={c.value} value={c.value}>{c.label}</option>
                ))}
              </select>
            </label>
          </div>
        </>
      )}
    </article>
  );
}

const rejectReasons: Record<string, string[]> = {
  absence: ['В эти даты релиз — выбери другой день', 'Обсудим на встрече 1:1', 'Сначала закрой просроченные шаги плана'],
  access: ['Хватит уровня «чтение»', 'Для задачи нужен staging, не прод', 'Обсудим с наставником, зачем нужен доступ'],
};

/** Отказ: причина обязательна, частые причины — в один клик. */
export function RejectForm(p: { kind: string; reason: string; onReason: (v: string) => void; pending: boolean; onSubmit: () => void; onCancel: () => void }) {
  const ok = p.reason.trim().length >= 3;
  return (
    <form
      className="reject-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (ok) p.onSubmit();
      }}
    >
      <label className="field">
        <span className="small">Причина отказа — стажёр увидит её в уведомлении</span>
        <textarea
          value={p.reason}
          onChange={(e) => p.onReason(e.target.value)}
          rows={2}
          required
          minLength={3}
          autoFocus
          placeholder="Например: в эти даты релиз, предложи другой день"
        />
      </label>
      <div className="reason-chips" aria-label="Частые причины">
        {rejectReasons[p.kind]?.map((r) => (
          <button key={r} type="button" className="chip" onClick={() => p.onReason(r)}>{r}</button>
        ))}
      </div>
      <div className="note-actions">
        <button className="btn danger small" disabled={p.pending || !ok}>Отказать</button>
        <button type="button" className="link small" onClick={p.onCancel}>Отмена</button>
      </div>
    </form>
  );
}
