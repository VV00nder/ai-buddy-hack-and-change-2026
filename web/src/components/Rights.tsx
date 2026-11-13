import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { keys } from '../hooks/queries';
import { initials, roleName } from '../lib/format';
import { useUi } from '../store/ui';

const rights: { perm: string; title: string; hint: string }[] = [
  { perm: 'kb', title: 'Инструкции и тесты', hint: 'Добавляет и правит инструкции, папки и тесты команды' },
  { perm: 'approvals', title: 'Согласование заявок', hint: 'Согласует отгулы и доступы вместо вас — например, на время отпуска' },
];

/** Раздел «Права»: руководитель передаёт часть своих полномочий сотрудникам команды. */
export function Rights() {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const q = useQuery({ queryKey: keys.rights, queryFn: api.rights });
  const m = useMutation({
    mutationFn: (v: { userId: string; perm: string; enabled: boolean }) => api.setRight(v.userId, v.perm, v.enabled),
    onSuccess: (r, v) => {
      qc.setQueryData(keys.rights, r);
      toast(v.enabled ? 'Право выдано — сотрудник получил уведомление' : 'Право отозвано');
    },
    onError: (e) => toast(e.message, 'error'),
  });
  if (q.isPending) return <p className="muted">Загружаю команду…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  return (
    <section className="rights">
      <p className="muted">Права действуют только в вашей команде. Стажёрам они не передаются. Решение заместителя по заявке видно в истории как его собственное.</p>
      {q.data.members.length === 0 && <p className="muted">В команде нет сотрудников, которым можно передать права.</p>}
      <table>
        <thead>
          <tr>
            <th scope="col">Сотрудник</th>
            {rights.map((r) => (
              <th key={r.perm} scope="col">
                {r.title}
                <span className="muted small th-hint">{r.hint}</span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {q.data.members.map((mem) => (
            <tr key={mem.user.id}>
              <td>
                <span className="person">
                  <span className={`avatar role-${mem.user.role}`} aria-hidden="true">{initials(mem.user.name)}</span>
                  <span>
                    {mem.user.name}
                    <br />
                    <span className="muted small">{roleName[mem.user.role]}</span>
                  </span>
                </span>
              </td>
              {rights.map((r) => {
                const on = mem.perms.includes(r.perm);
                return (
                  <td key={r.perm}>
                    <label className="switch">
                      <input
                        type="checkbox"
                        role="switch"
                        checked={on}
                        disabled={m.isPending}
                        onChange={() => m.mutate({ userId: mem.user.id, perm: r.perm, enabled: !on })}
                        aria-label={`${r.title}: ${mem.user.name}`}
                      />
                      <span className="small">{on ? 'выдано' : 'нет'}</span>
                    </label>
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
