import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { Approval } from '../api/schemas';
import { keys } from '../hooks/queries';
import { remindAt, remindChoices, waited } from '../lib/format';
import { useUi } from '../store/ui';

export function Approvals() {
  const q = useQuery({ queryKey: keys.approvals, queryFn: api.approvals });
  if (q.isPending) return <p className="muted">Загружаю заявки…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  const list = q.data.approvals;
  return (
    <section className="approvals">
      {list.length === 0 && (
        <div className="empty">
          <h2>Нечего согласовывать</h2>
          <p className="muted">Когда стажёр отправит заявку на отгул или доступ, она появится здесь и в уведомлениях.</p>
        </div>
      )}
      {list.map((a) => (
        <ApprovalRow key={a.externalId} a={a} />
      ))}
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
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: keys.approvals });
    void qc.invalidateQueries({ queryKey: keys.dashboard });
    void qc.invalidateQueries({ queryKey: ['internRequests'] });
  };
  const m = useMutation({
    mutationFn: (approve: boolean) => api.decide(a.kind, a.externalId, approve, comment),
    onSuccess: (_, approve) => {
      toast(approve ? 'Согласовано — стажёр получил уведомление' : 'Отклонено — стажёр увидит комментарий');
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
      {withComment ? (
        <label className="field">
          <span className="small">Комментарий стажёру</span>
          <input value={comment} onChange={(e) => setComment(e.target.value)} maxLength={500} placeholder="Необязательно" />
        </label>
      ) : null}
      <div className="note-actions">
        <button className="btn small" disabled={m.isPending} onClick={() => m.mutate(true)}>{d.exception ? 'Сделать исключение' : 'Согласовать'}</button>
        <button className="btn ghost small" disabled={m.isPending} onClick={() => m.mutate(false)}>Отклонить</button>
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
    </article>
  );
}
