import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { AttemptView } from '../api/schemas';
import { keys } from '../hooks/queries';
import { dayMonth, plural, time } from '../lib/format';
import { useUi } from '../store/ui';

/** Попытки по тестам, сгруппированные по тесту (в порядке первой попытки). */
export function groupByQuiz(atts: AttemptView[]): { quizId: string; title: string; attempts: AttemptView[]; best: number; passed: boolean }[] {
  const groups = new Map<string, { quizId: string; title: string; attempts: AttemptView[]; best: number; passed: boolean }>();
  for (const a of atts) {
    const g = groups.get(a.quizId) ?? { quizId: a.quizId, title: a.title, attempts: [], best: 0, passed: false };
    g.attempts.push(a);
    g.best = Math.max(g.best, a.percent);
    g.passed = g.passed || a.passed;
    groups.set(a.quizId, g);
  }
  return [...groups.values()];
}

const when = (iso: string) => `${dayMonth(iso)}, ${time(iso)}`;

/** Ответы стажёра по вопросам одной попытки. */
export function AttemptAnswers({ a }: { a: AttemptView }) {
  if (a.review.length === 0) return <p className="small muted">Тест изменён или удалён — разбор по вопросам недоступен.</p>;
  return (
    <ol className="qr-answers">
      {a.review.map((r, i) => (
        <li key={i} className={r.right ? 'right' : 'wrong'}>
          <p className="small qr-q">{r.text}</p>
          <p className="small">
            <span className={r.right ? 'pass-text' : 'overdue-text'}>{r.right ? 'Верно' : 'Ошибка'}</span>: {r.options[r.chosen] ?? 'нет ответа'}
          </p>
          {!r.right && r.correct >= 0 && <p className="small muted">Правильный ответ: {r.options[r.correct]}</p>}
          {!r.right && r.docTitle && <p className="small muted">Перечитать: {r.docTitle}</p>}
        </li>
      ))}
    </ol>
  );
}

/** Подробные результаты тестов стажёра: все попытки и ответы по каждому вопросу. */
export function QuizResultsPanel({ internId }: { internId: string }) {
  const res = useQuery({ queryKey: keys.quizResults(internId), queryFn: () => api.quizResults(internId) });
  const atts = res.data?.attempts ?? [];
  if (res.isPending) return null;
  return (
    <section className="quiz-results" aria-label="Результаты тестов">
      <h3>Результаты тестов</h3>
      {atts.length === 0 && <p className="small muted">Стажёр ещё не проходил тесты.</p>}
      {groupByQuiz(atts).map((g) => (
        <div key={g.quizId} className="qr-quiz">
          <div className="row">
            <strong>{g.title}</strong>
            <span className="small">
              <span className={g.passed ? 'pass-text' : 'overdue-text'}>{g.passed ? 'сдан' : 'не сдан'}</span>, лучший результат{' '}
              <span className="num">{g.best}%</span>, {g.attempts.length} {plural(g.attempts.length, 'попытка', 'попытки', 'попыток')}
            </span>
          </div>
          <ul className="qr-attempts">
            {g.attempts.map((a, i) => (
              <li key={a.id}>
                <details open={i === g.attempts.length - 1}>
                  <summary className="small">
                    Попытка {i + 1} · {when(a.at)} · <span className="num">{a.percent}%</span> ({a.correct} из {a.total}) ·{' '}
                    <span className={a.passed ? 'pass-text' : 'overdue-text'}>{a.passed ? 'сдан' : 'не сдан'}</span>
                  </summary>
                  <AttemptAnswers a={a} />
                </details>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </section>
  );
}

/** Сводка по стажёрам: лучший результат по каждому тесту. Клик — карточка стажёра с разбором. */
export function TeamQuizResults() {
  const go = useUi((s) => s.go);
  const res = useQuery({ queryKey: keys.teamQuizResults, queryFn: api.teamQuizResults });
  if (res.isPending) return <p className="muted">Загружаю результаты…</p>;
  if (res.isError) return <p className="error">{res.error.message}</p>;
  const interns = res.data.interns;
  const quizzes = new Map<string, string>();
  for (const i of interns) for (const a of i.attempts) quizzes.set(a.quizId, a.title);
  if (interns.length === 0) return null;
  return (
    <section className="team-quiz" aria-label="Результаты стажёров">
      <h3>Результаты стажёров</h3>
      {quizzes.size === 0 ? (
        <p className="small muted">Стажёры ещё не проходили тесты.</p>
      ) : (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">Стажёр</th>
                {[...quizzes].map(([id, title]) => (
                  <th key={id} scope="col">{title}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {interns.map((i) => {
                const groups = new Map(groupByQuiz(i.attempts).map((g) => [g.quizId, g]));
                return (
                  <tr key={i.intern.id} onClick={() => go('team', { internId: i.intern.id })}>
                    <td>
                      <button className="link" onClick={() => go('team', { internId: i.intern.id })}>{i.intern.name}</button>
                    </td>
                    {[...quizzes.keys()].map((id) => {
                      const g = groups.get(id);
                      return (
                        <td key={id} className="small">
                          {g ? (
                            <>
                              <span className={g.passed ? 'pass-text' : 'overdue-text'}>{g.passed ? 'сдан' : 'не сдан'}</span>{' '}
                              <span className="num">{g.best}%</span>
                              <span className="muted"> · {g.attempts.length} {plural(g.attempts.length, 'попытка', 'попытки', 'попыток')}</span>
                            </>
                          ) : (
                            <span className="muted">—</span>
                          )}
                        </td>
                      );
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <p className="small muted">Нажмите на стажёра, чтобы увидеть все попытки и ответы на каждый вопрос.</p>
    </section>
  );
}
