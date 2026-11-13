import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import type { User } from '../api/schemas';
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
      <Department />
      <h3 className="section-title">Права сотрудников</h3>
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

/** Сотрудники отдела руководителя и перевод в другой отдел с новым руководителем. */
export function Department() {
  const q = useQuery({ queryKey: keys.department, queryFn: api.department });
  if (q.isPending) return <p className="muted">Загружаю отдел…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  const { members, teams } = q.data;
  return (
    <section className="department" aria-label="Сотрудники отдела">
      <h3 className="section-title">Сотрудники отдела</h3>
      <p className="muted small">
        Перевод в другой отдел: руководителем сотрудника станет руководитель нового отдела, стажёр получит наставника нового отдела. Заявки,
        которые ждали вашего решения, перейдут новому руководителю.
      </p>
      {members.length === 0 && <p className="muted">В отделе пока нет сотрудников.</p>}
      <table>
        <thead>
          <tr>
            <th scope="col">Сотрудник</th>
            <th scope="col">Почта</th>
            <th scope="col">Перевести в отдел</th>
          </tr>
        </thead>
        <tbody>
          {members.map((u) => (
            <MemberRow key={u.id} u={u} teams={teams.filter((t) => t.id !== u.team)} />
          ))}
        </tbody>
      </table>
    </section>
  );
}

function MemberRow({ u, teams }: { u: User; teams: { id: string; name: string; leadName: string }[] }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [team, setTeam] = useState('');
  const m = useMutation({
    mutationFn: () => api.transfer(u.id, team),
    onSuccess: (d) => {
      qc.setQueryData(keys.department, d);
      void qc.invalidateQueries({ queryKey: keys.team });
      void qc.invalidateQueries({ queryKey: keys.rights });
      const t = teams.find((x) => x.id === team);
      toast(`${u.name} переведён(а) в «${t?.name}», руководитель — ${t?.leadName}`);
    },
    onError: (e) => toast(e.message, 'error'),
  });
  return (
    <tr>
      <td>
        <span className="person">
          <span className={`avatar role-${u.role}`} aria-hidden="true">{initials(u.name)}</span>
          <span>
            {u.name}
            <br />
            <span className="muted small">
              {roleName[u.role]}
              {u.registered && ' · зарегистрирован(а)'}
            </span>
          </span>
        </span>
      </td>
      <td className="small">{u.email ?? <span className="muted">—</span>}</td>
      <td>
        <div className="transfer">
          <label className="sr-only" htmlFor={`tr-${u.id}`}>Новый отдел для {u.name}</label>
          <select id={`tr-${u.id}`} value={team} onChange={(e) => setTeam(e.target.value)}>
            <option value="">Выберите отдел…</option>
            {teams.map((t) => (
              <option key={t.id} value={t.id}>{t.name} — {t.leadName}</option>
            ))}
          </select>
          <button
            className="btn ghost small"
            disabled={!team || m.isPending}
            onClick={() => {
              const t = teams.find((x) => x.id === team);
              if (typeof window === 'undefined' || window.confirm(`Перевести ${u.name} в «${t?.name}»? Руководителем станет ${t?.leadName}.`)) m.mutate();
            }}
          >
            Перевести
          </button>
        </div>
      </td>
    </tr>
  );
}
