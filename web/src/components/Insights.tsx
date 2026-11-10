import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Insights as T } from '../api/schemas';
import { keys } from '../hooks/queries';

export function Insights() {
  const q = useQuery({ queryKey: keys.insights, queryFn: api.insights, refetchInterval: 15_000 });
  if (q.isPending) return <p className="muted">Считаю метрики…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  return <InsightsBody d={q.data} />;
}

export function InsightsBody({ d }: { d: T }) {
  const hours = Math.round((d.mentorMinutesSaved / 60) * 10) / 10;
  const max = Math.max(1, ...d.byDay.map((x) => x.questions));
  return (
    <section className="insights">
      <div className="saved">
        <p className="saved-num">{String(hours).replace('.', ',')} ч</p>
        <p>
          времени наставников сэкономлено: {d.questions - d.escalations} вопросов решено без наставника и {d.actionsSubmitted} заявок оформлено с помощью AI-buddy.
          <span className="muted small"> Оценка: 10 минут на рутинный вопрос, 15 минут на помощь с заявкой.</span>
        </p>
      </div>
      <dl className="kpis">
        <div><dt>Вопросов без наставника</dt><dd>{d.selfServeRate}%</dd><p className="muted small">{d.questions} всего, {d.escalations} передано наставнику</p></div>
        <div><dt>Ответ помог</dt><dd>{d.feedbacks ? `${d.helpfulRate}%` : 'нет оценок'}</dd><p className="muted small">{d.feedbacks} оценок стажёров</p></div>
        <div><dt>Среднее время ответа</dt><dd>{d.avgAnswerSeconds < 0.1 ? 'меньше 0,1 с' : `${String(d.avgAnswerSeconds).replace('.', ',')} с`}</dd><p className="muted small">с учётом обращений к сервисам</p></div>
        <div><dt>Заявок выполнено</dt><dd>{d.actionsCompleted}</dd><p className="muted small">из {d.actionsSubmitted} отправленных</p></div>
        <div><dt>Шагов плана закрыто</dt><dd>{d.stepsCompleted}</dd><p className="muted small">в том числе автоматически</p></div>
        <div><dt>Ответов наставников</dt><dd>{d.mentorAnswers}</dd><p className="muted small">пополняют базу знаний</p></div>
      </dl>
      {d.byDay.length > 0 && (
        <figure className="bars">
          <figcaption>Вопросы по дням: всего и переданные наставнику</figcaption>
          <div className="bars-row">
            {d.byDay.map((x) => (
              <div key={x.day} className="bar" title={`${x.day}: ${x.questions} вопросов, ${x.escalations} наставнику`}>
                <span className="bar-total" style={{ height: `${(x.questions / max) * 100}%` }}>
                  <span className="bar-esc" style={{ height: `${x.questions ? (x.escalations / x.questions) * 100 : 0}%` }} />
                </span>
                <span className="small muted">{x.day.slice(8)}.{x.day.slice(5, 7)}</span>
              </div>
            ))}
          </div>
        </figure>
      )}
      <p className="muted small method">
        Как проверить эффект: включаем AI-buddy флагом для половины потока стажёров и сравниваем с контрольной группой время до первой самостоятельной задачи, долю шагов плана, закрытых в срок, и часы наставника на стажёра.
      </p>
    </section>
  );
}
