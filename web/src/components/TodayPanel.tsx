import type { StepView } from '../api/schemas';
import { usePlan, useSetStep } from '../hooks/queries';
import { useUi } from '../store/ui';

/** Панель «Сегодня» рядом с чатом: прогресс, следующий шаг и шаги этапа,
 *  которые можно отметить прямо здесь, не уходя из диалога. */
export function TodayPanel() {
  const plan = usePlan();
  const { go, setDraft } = useUi();
  const set = useSetStep();
  const p = plan.data;
  if (!p) return <aside className="today" aria-busy={plan.isPending}>{plan.isError && <p className="error">План не загрузился</p>}</aside>;
  const period = p.periods.find((x) => x.id === p.currentPeriod);
  const toggle = (s: StepView) => set.mutate({ stepId: s.id, status: s.status === 'done' ? 'todo' : 'done' });
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
      <div className="progress-line">
        <div className="progress" role="progressbar" aria-valuenow={p.percent} aria-valuemin={0} aria-valuemax={100} aria-label="Прогресс плана">
          <span style={{ width: `${p.percent}%` }} />
        </div>
        <span className="small num pct">{p.percent}%</span>
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
          <div className="row-start">
            {p.next.quiz ? (
              <button className="btn small" onClick={() => go('quizzes', { quizId: p.next!.quiz! })}>Пройти тест</button>
            ) : (
              <button className="btn small" disabled={set.isPending} onClick={() => toggle(p.next!)}>Готово</button>
            )}
            {p.next.prompt && (
              <button className="btn ghost small" onClick={() => setDraft(p.next!.prompt!)}>
                Спросить: «{p.next.prompt}»
              </button>
            )}
          </div>
        </div>
      )}
      {period && (
        <div className="period-mini">
          <h2>Этот этап</h2>
          <ul>
            {period.steps
              .filter((s) => s.owner === 'intern')
              .map((s) => (
                <li key={s.id} className={`mini-${s.status}`}>
                  {s.quiz && s.status !== 'done' ? (
                    <button className="mini-check quiz" onClick={() => go('quizzes', { quizId: s.quiz! })} aria-label={`Пройти тест: ${s.title}`} />
                  ) : (
                    <button
                      className="mini-check"
                      role="checkbox"
                      aria-checked={s.status === 'done'}
                      aria-label={s.status === 'done' ? `Вернуть в работу: ${s.title}` : `Отметить выполненным: ${s.title}`}
                      disabled={set.isPending || !!s.quiz}
                      onClick={() => toggle(s)}
                    />
                  )}
                  <span>{s.title}</span>
                </li>
              ))}
          </ul>
          <button className="link" onClick={() => go('plan')}>Открыть план целиком</button>
        </div>
      )}
    </aside>
  );
}
