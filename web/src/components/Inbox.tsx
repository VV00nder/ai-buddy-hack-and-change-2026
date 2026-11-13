import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { Question } from '../api/schemas';
import { keys } from '../hooks/queries';
import { relative } from '../lib/format';
import { useUi } from '../store/ui';

export function Inbox() {
  const q = useQuery({ queryKey: keys.inbox, queryFn: api.inbox });
  const team = useQuery({ queryKey: keys.team, queryFn: api.team });
  const known = new Map((team.data?.interns ?? []).map((i) => [i.user.id, i.user.name]));
  const nameOf = (x: Question) => x.internName ?? known.get(x.internId) ?? 'Стажёр';
  const list = q.data?.questions ?? [];
  const open = list.filter((x) => x.status === 'open');
  const answered = list.filter((x) => x.status === 'answered');
  if (q.isPending) return <p className="muted">Загружаю вопросы…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  return (
    <section className="inbox">
      <p className="muted">
        Сюда приходят вопросы ваших стажёров, на которые AI-buddy не нашёл ответа в инструкциях. Ответить может любой наставник стажёра, его руководитель или администратор. Ответ уйдёт стажёру в чат, а если сохранить его в базу знаний, следующий стажёр получит его без вашего участия.
      </p>
      {open.length === 0 && <div className="empty"><h2>Новых вопросов нет</h2><p className="muted">AI-buddy справляется сам.</p></div>}
      {open.map((x) => (
        <AnswerForm key={x.id} q={x} who={nameOf(x)} />
      ))}
      {answered.length > 0 && (
        <>
          <h2 className="section-title">Отвечено</h2>
          <ul className="answered">
            {answered.map((x) => (
              <li key={x.id}>
                <p><strong>{nameOf(x)}:</strong> {x.text}</p>
                <p className="muted">{x.answer}</p>
                {x.answeredBy && <p className="small muted">Ответил(а): {x.answeredBy}</p>}
                {x.addToFaq && <span className="tag">в базе знаний</span>}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

export function AnswerForm({ q, who, compact = false }: { q: Question; who: string; compact?: boolean }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [answer, setAnswer] = useState('');
  const [faq, setFaq] = useState(true);
  const m = useMutation({
    mutationFn: () => api.answer(q.id, answer, faq),
    onSuccess: () => {
      toast(faq ? 'Ответ отправлен и сохранён в базу знаний' : 'Ответ отправлен стажёру');
      void qc.invalidateQueries({ queryKey: keys.inbox });
      void qc.invalidateQueries({ queryKey: ['internRequests'] });
      void qc.invalidateQueries({ queryKey: keys.dashboard });
    },
    onError: (e) => toast(e.message, 'error'),
  });
  return (
    <article className={`question ${compact ? 'compact' : ''}`}>
      {!compact && <p className="muted small">{who}, {relative(q.createdAt)}</p>}
      {!compact && <p className="question-text">{q.text}</p>}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          m.mutate();
        }}
      >
        <label className="sr-only" htmlFor={`a-${q.id}`}>Ответ</label>
        <textarea id={`a-${q.id}`} value={answer} onChange={(e) => setAnswer(e.target.value)} rows={compact ? 2 : 3} placeholder="Ответ стажёру" required minLength={2} />
        <div className="row">
          <label className="checkbox">
            <input type="checkbox" checked={faq} onChange={(e) => setFaq(e.target.checked)} />
            Сохранить в базу знаний команды
          </label>
          <button className="btn" disabled={m.isPending || answer.trim().length < 2}>Ответить</button>
        </div>
      </form>
    </article>
  );
}
