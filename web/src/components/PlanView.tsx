import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Plan, StepStatus, StepView } from '../api/schemas';
import { keys, usePlan, useSend, useSetStep } from '../hooks/queries';
import { dayMonth, stepStatusName } from '../lib/format';
import { useUi } from '../store/ui';

/** План стажёра. Без internId — свой план; с internId — план стажёра для наставника/руководителя. */
export function PlanView({ internId }: { internId?: string }) {
  const own = usePlan();
  const other = useQuery({ queryKey: keys.internPlan(internId ?? ''), queryFn: () => api.internPlan(internId!), enabled: !!internId });
  const q = internId ? other : own;
  const plan: Plan | undefined = internId ? other.data?.plan : own.data;
  if (q.isPending) return <p className="muted">Загружаю план…</p>;
  if (q.isError || !plan) return <p className="error">План не загрузился: {q.error?.message}</p>;
  return <PlanBody plan={plan} internId={internId} internName={internId ? other.data?.intern.name : undefined} />;
}

export function PlanBody({ plan, internId, internName }: { plan: Plan; internId?: string; internName?: string }) {
  return (
    <section className="plan">
      <header className="plan-head">
        <div>
          {internName && <p className="muted">План стажёра</p>}
          <h2>{internName ?? `День ${plan.day}`}</h2>
          <p className="muted">
            {internName ? `${plan.day}-й день, ` : ''}выполнено {plan.done} из {plan.total} шагов
            {plan.overdue > 0 ? `, просрочено ${plan.overdue}` : ''}
          </p>
        </div>
        <div className="ring" style={{ ['--p' as string]: plan.percent }} aria-label={`Прогресс ${plan.percent}%`}>
          <span>{plan.percent}%</span>
        </div>
      </header>
      <ol className="periods">
        {plan.periods.map((per) => (
          <li key={per.id} className={`period period-${per.state}`}>
            <div className="period-head">
              <h3>{per.name}</h3>
              <span className="muted small">
                {per.done}/{per.total}
                {per.state === 'current' ? ' — сейчас' : ''}
              </span>
            </div>
            <ul className="steps">
              {per.steps.map((s) => (
                <StepRow key={s.id} s={s} internId={internId} isNext={plan.next?.id === s.id} />
              ))}
            </ul>
          </li>
        ))}
      </ol>
    </section>
  );
}

const cycle: Record<StepStatus, StepStatus> = { todo: 'done', in_progress: 'done', blocked: 'done', done: 'todo' };

export function StepRow({ s, internId, isNext }: { s: StepView; internId?: string; isNext: boolean }) {
  const set = useSetStep();
  const send = useSend();
  const go = useUi((st) => st.go);
  const canEdit = internId ? true : s.owner === 'intern';
  return (
    <li className={`step step-${s.status} ${s.overdue ? 'overdue' : ''} ${isNext ? 'is-next' : ''}`}>
      <button
        className="check"
        disabled={!canEdit || set.isPending}
        aria-label={s.status === 'done' ? `Вернуть шаг «${s.title}» в работу` : `Отметить шаг «${s.title}» выполненным`}
        aria-pressed={s.status === 'done'}
        onClick={() => set.mutate({ stepId: s.id, status: cycle[s.status], internId })}
      />
      <div className="step-body">
        <p className="step-title">
          {s.title}
          {isNext && <span className="tag next-tag">следующий</span>}
          {s.owner === 'mentor' && <span className="tag">делает наставник</span>}
          {s.overdue && <span className="tag overdue-tag">просрочен с {dayMonth(s.due)}</span>}
        </p>
        <p className="muted small">{s.description}</p>
        {s.note && <p className="small step-note">{s.note}</p>}
        <div className="step-actions">
          <span className="muted small">{s.category}, {stepStatusName[s.status]}</span>
          {canEdit && s.status !== 'done' && s.status !== 'in_progress' && (
            <button className="link small" onClick={() => set.mutate({ stepId: s.id, status: 'in_progress', internId })}>Взять в работу</button>
          )}
          {canEdit && s.status !== 'done' && s.status !== 'blocked' && (
            <button className="link small" onClick={() => set.mutate({ stepId: s.id, status: 'blocked', internId, note: 'Нужна помощь' })}>Есть блокер</button>
          )}
          {!internId && s.prompt && s.status !== 'done' && (
            <button
              className="link small"
              onClick={() => {
                send.mutate(s.prompt!);
                go('chat');
              }}
            >
              Спросить buddy
            </button>
          )}
        </div>
      </div>
    </li>
  );
}
