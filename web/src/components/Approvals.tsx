import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { Approval } from '../api/schemas';
import { keys } from '../hooks/queries';
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

export function ApprovalRow({ a }: { a: Approval }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [comment, setComment] = useState('');
  const m = useMutation({
    mutationFn: (approve: boolean) => api.decide(a.kind, a.externalId, approve, comment),
    onSuccess: (_, approve) => {
      toast(approve ? 'Согласовано — стажёр получит уведомление' : 'Отклонено — стажёр увидит комментарий');
      void qc.invalidateQueries({ queryKey: keys.approvals });
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const d = a.details as Record<string, string | undefined>;
  return (
    <article className={`note note-${a.kind} approval`}>
      <header>
        <span className="note-kind">{a.userName}</span>
        <span className="muted small">{a.externalId}</span>
      </header>
      <h3>{a.title}</h3>
      {(d.reason || d.justification) && <p className="small">«{d.reason ?? d.justification}»</p>}
      <label className="field">
        <span>Комментарий стажёру</span>
        <input value={comment} onChange={(e) => setComment(e.target.value)} maxLength={500} placeholder="Необязательно" />
      </label>
      <div className="note-actions">
        <button className="btn" disabled={m.isPending} onClick={() => m.mutate(true)}>Согласовать</button>
        <button className="btn ghost" disabled={m.isPending} onClick={() => m.mutate(false)}>Отклонить</button>
      </div>
    </article>
  );
}
