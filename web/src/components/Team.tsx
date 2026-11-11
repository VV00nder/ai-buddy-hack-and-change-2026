import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { keys } from '../hooks/queries';
import { initials } from '../lib/format';
import { useUi } from '../store/ui';
import { PlanView } from './PlanView';

export function Team() {
  const { internId, go } = useUi();
  const q = useQuery({ queryKey: keys.team, queryFn: api.team });
  if (internId)
    return (
      <div>
        <button className="link back" onClick={() => go('team')}>Все стажёры</button>
        <PlanView internId={internId} />
      </div>
    );
  if (q.isPending) return <p className="muted">Загружаю…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  if (q.data.interns.length === 0) return <div className="empty"><h2>У вас нет стажёров</h2></div>;
  return (
    <section className="team">
      <table>
        <thead>
          <tr>
            <th scope="col">Стажёр</th>
            <th scope="col">День</th>
            <th scope="col">Прогресс</th>
            <th scope="col">Цели</th>
            <th scope="col">Следующий шаг</th>
            <th scope="col">Вопросы</th>
          </tr>
        </thead>
        <tbody>
          {q.data.interns.map((i) => (
            <tr key={i.user.id} onClick={() => go('team', { internId: i.user.id })}>
              <td>
                <button className="person" onClick={() => go('team', { internId: i.user.id })}>
                  <span className="avatar role-intern" aria-hidden="true">{initials(i.user.name)}</span>
                  <span>
                    {i.user.name}
                    <br />
                    <span className="muted small">{i.user.teamName}</span>
                  </span>
                </button>
              </td>
              <td>{i.day}</td>
              <td>
                <div className="progress" aria-label={`${i.percent}%`}><span style={{ width: `${i.percent}%` }} /></div>
                <span className="small">{i.done}/{i.total}{i.overdue > 0 && <span className="overdue-text">, просрочено {i.overdue}</span>}</span>
              </td>
              <td className="small">{i.goalsTotal > 0 ? `${i.goalsDone} из ${i.goalsTotal}` : <span className="overdue-text">не поставлены</span>}</td>
              <td className="small">{i.nextStep ?? 'Всё выполнено'}</td>
              <td>{i.openQuestions > 0 ? <span className="tag overdue-tag">{i.openQuestions} без ответа</span> : <span className="muted">—</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
