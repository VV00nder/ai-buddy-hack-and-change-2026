import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { Goal } from '../api/schemas';
import { keys } from '../hooks/queries';
import { dayMonth } from '../lib/format';
import { useUi } from '../store/ui';

const statusName: Record<Goal['status'], string> = { agreed: 'Согласована', in_progress: 'В работе', done: 'Выполнена' };

/** Цели стажировки: стажёр отмечает прогресс, руководитель и наставник ставят цели. */
export function Goals({ internId, canAdd }: { internId?: string; canAdd: boolean }) {
  const q = useQuery({ queryKey: keys.goals(internId), queryFn: () => api.goals(internId) });
  const goals = q.data?.goals ?? [];
  return (
    <section className="goals" aria-labelledby="goals-title">
      <div className="goals-head">
        <h3 id="goals-title">Цели стажировки</h3>
        <span className="muted small">{goals.filter((g) => g.status === 'done').length} из {goals.length} выполнено</span>
      </div>
      {q.isError && <p className="error">Цели не загрузились</p>}
      {q.isSuccess && goals.length === 0 && (
        <p className="muted">
          {canAdd ? 'Поставьте стажёру 2–3 измеримые цели на период адаптации.' : 'Цели пока не поставлены — их согласует руководитель на первой неделе.'}
        </p>
      )}
      <ul className="goal-list">
        {goals.map((g) => (
          <GoalRow key={g.id} g={g} internId={internId} />
        ))}
      </ul>
      {canAdd && internId && <AddGoal internId={internId} />}
    </section>
  );
}

export function GoalRow({ g, internId }: { g: Goal; internId?: string }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [progress, setProgress] = useState(g.progress);
  const m = useMutation({
    mutationFn: (patch: Parameters<typeof api.updateGoal>[1]) => api.updateGoal(g.id, patch),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.goals(internId) }),
    onError: (e) => toast(e.message, 'error'),
  });
  return (
    <li className={`goal goal-${g.status}`}>
      <div className="goal-main">
        <p className="goal-title">{g.title}</p>
        <p className="small muted">Критерий успеха: {g.metric}</p>
        {g.note && <p className="small step-note">{g.note}</p>}
      </div>
      <div className="goal-side">
        <span className="small">до {dayMonth(g.due)}, {statusName[g.status].toLowerCase()}</span>
        <label className="goal-progress">
          <span className="sr-only">Прогресс цели «{g.title}»</span>
          <input
            type="range"
            min={0}
            max={100}
            step={10}
            value={progress}
            disabled={g.status === 'done' || m.isPending}
            onChange={(e) => setProgress(Number(e.target.value))}
            onMouseUp={() => progress !== g.progress && m.mutate({ progress })}
            onKeyUp={() => progress !== g.progress && m.mutate({ progress })}
            onTouchEnd={() => progress !== g.progress && m.mutate({ progress })}
          />
          <span className="small">{progress}%</span>
        </label>
        {g.status !== 'done' ? (
          <button className="link small" disabled={m.isPending} onClick={() => m.mutate({ status: 'done' })}>Отметить выполненной</button>
        ) : (
          <button className="link small" disabled={m.isPending} onClick={() => m.mutate({ status: 'in_progress', progress: 90 })}>Вернуть в работу</button>
        )}
      </div>
    </li>
  );
}

export function AddGoal({ internId }: { internId: string }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [title, setTitle] = useState('');
  const [metric, setMetric] = useState('');
  const [due, setDue] = useState('');
  const m = useMutation({
    mutationFn: () => api.addGoal(internId, title, metric, due),
    onSuccess: () => {
      setTitle('');
      setMetric('');
      setDue('');
      toast('Цель добавлена — стажёр получил уведомление');
      void qc.invalidateQueries({ queryKey: keys.goals(internId) });
      void qc.invalidateQueries({ queryKey: keys.team });
    },
    onError: (e) => toast(e.message, 'error'),
  });
  function submit(e: FormEvent) {
    e.preventDefault();
    m.mutate();
  }
  return (
    <form className="add-goal" onSubmit={submit}>
      <label className="field">
        <span>Цель</span>
        <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Например: закрыть первую задачу SEARCH-101" required minLength={5} maxLength={200} />
      </label>
      <label className="field">
        <span>Критерий успеха</span>
        <input value={metric} onChange={(e) => setMetric(e.target.value)} placeholder="Как поймём, что цель достигнута" required minLength={5} maxLength={300} />
      </label>
      <label className="field">
        <span>Срок</span>
        <input type="date" value={due} onChange={(e) => setDue(e.target.value)} required />
      </label>
      <button className="btn" disabled={m.isPending}>Добавить цель</button>
    </form>
  );
}
