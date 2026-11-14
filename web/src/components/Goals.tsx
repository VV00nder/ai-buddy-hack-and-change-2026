import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api, type TaskInput } from '../api/client';
import type { Goal, StepView } from '../api/schemas';
import { keys } from '../hooks/queries';
import { dayMonth } from '../lib/format';
import { useUi } from '../store/ui';
import { DeleteButton } from './DeleteButton';

const statusName: Record<Goal['status'], string> = { agreed: 'Согласована', in_progress: 'В работе', done: 'Выполнена' };

/** Цели стажировки. Прогресс цели считается сам — по закрытым задачам; задачи-шаги плана
 *  закрываются вместе с шагом. Руководитель и наставник ставят цели и добавляют задачи. */
export function Goals({ internId, canAdd, steps = [] }: { internId?: string; canAdd: boolean; steps?: StepView[] }) {
  const q = useQuery({ queryKey: keys.goals(internId), queryFn: () => api.goals(internId) });
  const goals = q.data?.goals ?? [];
  const [adding, setAdding] = useState(false);
  const stages = goals.filter((g) => g.auto);
  const own = goals.filter((g) => !g.auto);
  return (
    <section className="goals" aria-labelledby="goals-title">
      <div className="goals-head">
        <h3 id="goals-title">Цели стажировки</h3>
        <span className="muted small">{goals.filter((g) => g.status === 'done').length} из {goals.length} выполнено</span>
        {canAdd && internId && !adding && (
          <button className="link small" onClick={() => setAdding(true)}>+ цель</button>
        )}
      </div>
      {q.isError && <p className="error">Цели не загрузились</p>}
      {stages.length > 0 && (
        <>
          <p className="small muted">Этапы плана — цели создаются из плана сами, шкала заполняется по мере выполнения шагов.</p>
          <ul className="goal-list stage-goals">
            {stages.map((g) => (
              <StageGoal key={g.id} g={g} />
            ))}
          </ul>
        </>
      )}
      {q.isSuccess && goals.length === 0 && !canAdd && <p className="muted small">Целей пока нет — они появятся вместе с планом адаптации.</p>}
      {own.length > 0 && <p className="small muted">Цели, поставленные вручную:</p>}
      {q.isSuccess && own.length === 0 && canAdd && !adding && (
        <p className="muted small">Можно добавить свою цель: из шагов плана и своих задач — процент считается сам.</p>
      )}
      <ul className="goal-list">
        {own.map((g) => (
          <GoalRow key={g.id} g={g} internId={internId} canAdd={canAdd} steps={steps} />
        ))}
      </ul>
      {canAdd && internId && adding && <AddGoal internId={internId} steps={steps} onDone={() => setAdding(false)} />}
    </section>
  );
}

/** Цель этапа плана: компактно — шкала и число шагов, шаги раскрываются. */
export function StageGoal({ g }: { g: Goal }) {
  const done = g.tasks.filter((t) => t.done).length;
  return (
    <li className={`goal stage-goal goal-${g.status}`}>
      <details>
        <summary>
          <span className="goal-title">{g.title}</span>
          <span className="progress-line">
            <span className="progress" role="progressbar" aria-valuenow={g.progress} aria-valuemin={0} aria-valuemax={100} aria-label={`Прогресс: ${g.title}`}>
              <span style={{ width: `${g.progress}%` }} />
            </span>
            <span className="small num pct">{g.progress}%</span>
          </span>
          <span className="small muted">
            {done} из {g.tasks.length} шагов{g.due && ` · до ${dayMonth(g.due)}`}
          </span>
        </summary>
        <ul className="goal-tasks">
          {g.tasks.map((t) => (
            <li key={t.id} className={t.done ? 'done' : ''}>
              <span aria-hidden="true">{t.done ? '✓' : '○'}</span> <span>{t.title}</span>
            </li>
          ))}
        </ul>
      </details>
    </li>
  );
}

export function GoalRow({ g, internId, canAdd = false, steps = [] }: { g: Goal; internId?: string; canAdd?: boolean; steps?: StepView[] }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [task, setTask] = useState('');
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: keys.goals(internId) });
    void qc.invalidateQueries({ queryKey: keys.team });
    void qc.invalidateQueries({ queryKey: keys.dashboard });
  };
  const toggle = useMutation({
    mutationFn: (v: { id: string; done: boolean }) => api.setGoalTask(g.id, v.id, v.done),
    onSuccess: refresh,
    onError: (e) => toast(e.message, 'error'),
  });
  const add = useMutation({
    mutationFn: (t: TaskInput) => api.addGoalTask(g.id, t),
    onSuccess: () => {
      setTask('');
      refresh();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  // цель, поставленную вручную, руководитель, наставник или администратор может убрать
  const remove = useMutation({
    mutationFn: () => api.deleteGoal(g.id),
    onSuccess: () => {
      toast('Цель убрана — стажёр получил уведомление');
      refresh();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const stepTitle = (id?: string) => steps.find((s) => s.id === id)?.title;
  const done = g.tasks.filter((t) => t.done).length;
  return (
    <li className={`goal goal-${g.status}`}>
      <div className="goal-main">
        <div className="goal-top">
          <p className="goal-title">{g.title}</p>
          <span className="small muted goal-meta">
            до {dayMonth(g.due)} · {statusName[g.status].toLowerCase()}
          </span>
          {canAdd && !g.auto && (
            <DeleteButton what={`цель «${g.title}»`} label="Убрать" note="Убрать цель? Стажёр получит уведомление." pending={remove.isPending} onConfirm={() => remove.mutate()} />
          )}
        </div>
        <div className="progress-line">
          <div className="progress" role="progressbar" aria-valuenow={g.progress} aria-valuemin={0} aria-valuemax={100} aria-label={`Прогресс цели «${g.title}»`}>
            <span style={{ width: `${g.progress}%` }} />
          </div>
          <span className="small num pct">{g.progress}%</span>
          <span className="small muted">
            {done} из {g.tasks.length} задач
          </span>
        </div>
        <p className="small muted">Критерий успеха: {g.metric}</p>
        {g.note && <p className="small step-note">{g.note}</p>}
        <ul className="goal-tasks">
          {g.tasks.map((t) => (
            <li key={t.id} className={t.done ? 'done' : ''}>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={t.done}
                  // задача-шаг закрывается в плане: так прогресс плана и цели не расходятся
                  disabled={!!t.stepId || toggle.isPending}
                  onChange={() => toggle.mutate({ id: t.id, done: !t.done })}
                />
                <span>{t.title || stepTitle(t.stepId) || 'Шаг плана'}</span>
              </label>
              {t.stepId && <span className="tag small" title={stepTitle(t.stepId) ?? 'Шаг плана'}>шаг плана</span>}
            </li>
          ))}
        </ul>
        {canAdd && (
          <form
            className="goal-add-task"
            onSubmit={(e) => {
              e.preventDefault();
              if (task.trim()) add.mutate({ title: task.trim() });
            }}
          >
            <label className="sr-only" htmlFor={`gt-${g.id}`}>Новая задача цели «{g.title}»</label>
            <input id={`gt-${g.id}`} value={task} onChange={(e) => setTask(e.target.value)} placeholder="Добавить задачу" minLength={3} maxLength={200} />
            <button className="btn ghost small" disabled={add.isPending || task.trim().length < 3}>Добавить</button>
          </form>
        )}
      </div>
    </li>
  );
}

export function AddGoal({ internId, steps, onDone }: { internId: string; steps: StepView[]; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [title, setTitle] = useState('');
  const [metric, setMetric] = useState('');
  const [due, setDue] = useState('');
  const [picked, setPicked] = useState<string[]>([]);
  const [custom, setCustom] = useState<string[]>(['']);
  const tasks: TaskInput[] = [
    ...picked.map((id) => ({ title: steps.find((s) => s.id === id)?.title ?? id, stepId: id })),
    ...custom.map((t) => t.trim()).filter((t) => t.length > 0).map((t) => ({ title: t })),
  ];
  const m = useMutation({
    mutationFn: () => api.addGoal(internId, { title, metric, due, tasks }),
    onSuccess: () => {
      toast('Цель добавлена — стажёр получил уведомление');
      void qc.invalidateQueries({ queryKey: keys.goals(internId) });
      void qc.invalidateQueries({ queryKey: keys.team });
      void qc.invalidateQueries({ queryKey: keys.dashboard });
      onDone();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  function submit(e: FormEvent) {
    e.preventDefault();
    if (tasks.length === 0) {
      toast('Добавьте хотя бы одну задачу — по ним считается прогресс цели', 'error');
      return;
    }
    m.mutate();
  }
  const open = steps.filter((s) => s.owner === 'intern');
  return (
    <form className="add-goal" onSubmit={submit}>
      <div className="add-goal-grid">
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
      </div>
      <fieldset className="goal-task-pick">
        <legend className="small">Задачи цели — прогресс считается по закрытым</legend>
        {open.length > 0 && (
          <label className="field">
            <span className="small muted">Шаги плана (закроются вместе с шагом)</span>
            <select
              value=""
              onChange={(e) => e.target.value && !picked.includes(e.target.value) && setPicked([...picked, e.target.value])}
            >
              <option value="">Выберите шаг…</option>
              {open
                .filter((s) => !picked.includes(s.id))
                .map((s) => (
                  <option key={s.id} value={s.id}>{s.title}</option>
                ))}
            </select>
          </label>
        )}
        {picked.length > 0 && (
          <ul className="picked">
            {picked.map((id) => (
              <li key={id}>
                <span className="tag small">шаг плана</span> {steps.find((s) => s.id === id)?.title}
                <button type="button" className="link small" aria-label="Убрать шаг" onClick={() => setPicked(picked.filter((x) => x !== id))}>✕</button>
              </li>
            ))}
          </ul>
        )}
        {custom.map((t, i) => (
          <div key={i} className="opt-edit">
            <input
              value={t}
              onChange={(e) => setCustom(custom.map((x, k) => (k === i ? e.target.value : x)))}
              placeholder={`Своя задача ${i + 1}`}
              aria-label={`Своя задача ${i + 1}`}
              maxLength={200}
            />
            {custom.length > 1 && (
              <button type="button" className="link small" aria-label={`Удалить задачу ${i + 1}`} onClick={() => setCustom(custom.filter((_, k) => k !== i))}>✕</button>
            )}
          </div>
        ))}
        <button type="button" className="link small" onClick={() => setCustom([...custom, ''])}>+ своя задача</button>
      </fieldset>
      <div className="row-start">
        <button className="btn" disabled={m.isPending}>Добавить цель</button>
        <button type="button" className="link" onClick={onDone}>Отмена</button>
      </div>
    </form>
  );
}
