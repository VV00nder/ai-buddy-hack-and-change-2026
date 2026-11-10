import { usePlan } from '../hooks/queries';
import { useUi } from '../store/ui';

export function TodayPanel() {
  const plan = usePlan();
  const { go, setDraft } = useUi();
  const p = plan.data;
  if (!p) return <aside className="today" aria-busy={plan.isPending}>{plan.isError && <p className="error">План не загрузился</p>}</aside>;
  const period = p.periods.find((x) => x.id === p.currentPeriod);
  return (
    <aside className="today" aria-label="Сегодня">
      <p className="today-day">
        <span className="today-num">{p.day}</span>
        <span>
          день стажировки
          <br />
          <span className="muted">этап «{period?.name}»</span>
        </span>
      </p>
      <div className="progress" role="progressbar" aria-valuenow={p.percent} aria-valuemin={0} aria-valuemax={100} aria-label="Прогресс плана">
        <span style={{ width: `${p.percent}%` }} />
      </div>
      <p className="muted small">
        Выполнено {p.done} из {p.total}
        {p.overdue > 0 && <span className="overdue-text">, просрочено {p.overdue}</span>}
      </p>
      {p.next && (
        <div className="next">
          <h2>Следующий шаг</h2>
          <p className="next-title">{p.next.title}</p>
          <p className="muted small">{p.next.description}</p>
          {p.next.prompt && (
            <button className="btn ghost small" onClick={() => setDraft(p.next!.prompt!)}>
              Спросить: «{p.next.prompt}»
            </button>
          )}
        </div>
      )}
      {period && (
        <div className="period-mini">
          <h2>Этот этап</h2>
          <ul>
            {period.steps
              .filter((s) => s.owner === 'intern')
              .map((s) => (
                <li key={s.id} className={`mini-${s.status}`}>{s.title}</li>
              ))}
          </ul>
          <button className="link" onClick={() => go('plan')}>Открыть план целиком</button>
        </div>
      )}
    </aside>
  );
}
