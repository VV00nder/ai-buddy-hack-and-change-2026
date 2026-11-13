import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { User } from '../api/schemas';
import { keys, useLiveUpdates } from '../hooks/queries';
import { initials } from '../lib/format';

/** Новый сотрудник ждёт, пока руководитель примет его в отдел и назначит роль. */
export function Waiting({ user }: { user: User }) {
  const qc = useQueryClient();
  useLiveUpdates(true);
  // опрос на случай, если поток событий недоступен: как только руководитель назначит роль, откроется кабинет
  useQuery({ queryKey: keys.me, queryFn: api.me, refetchInterval: 5000 });
  async function logout() {
    await api.logout().catch(() => undefined);
    qc.clear();
    window.location.reload();
  }
  return (
    <main className="login waiting">
      <section className="login-panel" aria-labelledby="wait-title">
        <span className={`avatar role-newcomer big`} aria-hidden="true">{initials(user.name)}</span>
        <h2 id="wait-title">Учётная запись создана</h2>
        <p>
          {user.name}, руководитель получил уведомление и назначит вам отдел, роль и наставника. Эта страница обновится сама — план адаптации,
          чат и инструкции откроются сразу после назначения.
        </p>
        <p className="muted small">Почта для входа: {user.email}</p>
        <div className="row-start">
          <span className="spinner" aria-hidden="true" />
          <span className="muted small" role="status">Ждём назначения…</span>
        </div>
        <button className="link" onClick={() => void logout()}>Выйти</button>
      </section>
    </main>
  );
}
