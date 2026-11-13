import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import type { Department as Dept, TeamInfo, User } from '../api/schemas';
import { api } from '../api/client';
import { keys, useMe } from '../hooks/queries';
import { initials, roleName } from '../lib/format';
import { useUi } from '../store/ui';

const rights: { perm: string; title: string; hint: string }[] = [
  { perm: 'kb', title: 'Инструкции и тесты', hint: 'Добавляет и правит инструкции, папки и тесты команды' },
  { perm: 'approvals', title: 'Согласование заявок', hint: 'Согласует отгулы и доступы вместо вас — например, на время отпуска' },
];

/** Раздел «Права»: руководитель передаёт часть своих полномочий сотрудникам команды. */
export function Rights() {
  const admin = useMe().data?.user.role === 'admin';
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
      <p className="muted">
        {admin
          ? 'Права сотрудников всех отделов. «Согласование заявок» делает сотрудника заместителем руководителя его отдела. Стажёрам права не передаются.'
          : 'Права действуют только в вашей команде. Стажёрам они не передаются. Решение заместителя по заявке видно в истории как его собственное.'}
      </p>
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
                    <span className="muted small">
                      {roleName[mem.user.role]}
                      {admin && mem.user.teamName && ` · ${mem.user.teamName}`}
                    </span>
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
  const admin = useMe().data?.user.role === 'admin';
  const q = useQuery({ queryKey: keys.department, queryFn: api.department });
  if (q.isPending) return <p className="muted">Загружаю отдел…</p>;
  if (q.isError) return <p className="error">{q.error.message}</p>;
  if (admin) return <AdminDepartment d={q.data} />;
  const { members, teams, newcomers, mentors } = q.data;
  return (
    <section className="department" aria-label="Сотрудники отдела">
      <h3 className="section-title">Новые сотрудники</h3>
      {newcomers.length === 0 ? (
        <p className="muted small">Нет новых регистраций. Когда сотрудник зарегистрируется (ФИО, почта, пароль), он появится здесь — назначьте ему роль и наставника.</p>
      ) : (
        <>
          <p className="muted small">Сотрудник зарегистрировался и ждёт назначения. После принятия в отдел у стажёра откроется план адаптации с сегодняшнего дня.</p>
          <table className="compact-table">
            <thead>
              <tr>
                <th scope="col">Сотрудник</th>
                <th scope="col">Роль</th>
                <th scope="col">Наставник</th>
                <th scope="col"><span className="sr-only">Действие</span></th>
              </tr>
            </thead>
            <tbody>
              {newcomers.map((u) => (
                <NewcomerRow key={u.id} u={u} mentors={mentors} />
              ))}
            </tbody>
          </table>
        </>
      )}
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

const assignRoles: { value: string; label: string }[] = [
  { value: 'intern', label: 'Стажёр' },
  { value: 'engineer', label: 'Сотрудник' },
  { value: 'mentor', label: 'Наставник' },
];

/** Руководитель принимает зарегистрировавшегося сотрудника в отдел и назначает роль. */
export function NewcomerRow({ u, mentors }: { u: User; mentors: User[] }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [role, setRole] = useState('intern');
  const [mentor, setMentor] = useState(mentors[0]?.id ?? '');
  const m = useMutation({
    mutationFn: () => api.assign(u.id, role, role === 'intern' ? mentor : ''),
    onSuccess: (d) => {
      qc.setQueryData(keys.department, d);
      void qc.invalidateQueries({ queryKey: keys.team });
      void qc.invalidateQueries({ queryKey: keys.dashboard });
      void qc.invalidateQueries({ queryKey: keys.rights });
      toast(`${u.name} принят(а) в отдел — сотрудник получил уведомление`);
    },
    onError: (e) => toast(e.message, 'error'),
  });
  return (
    <tr>
      <td>
        <span className="person">
          <span className="avatar role-newcomer" aria-hidden="true">{initials(u.name)}</span>
          <span>
            {u.name}
            <br />
            <span className="muted small">{u.email}</span>
          </span>
        </span>
      </td>
      <td>
        <label className="sr-only" htmlFor={`role-${u.id}`}>Роль для {u.name}</label>
        <select id={`role-${u.id}`} value={role} onChange={(e) => setRole(e.target.value)}>
          {assignRoles.map((r) => (
            <option key={r.value} value={r.value}>{r.label}</option>
          ))}
        </select>
      </td>
      <td>
        {role === 'intern' ? (
          <>
            <label className="sr-only" htmlFor={`mentor-${u.id}`}>Наставник для {u.name}</label>
            <select id={`mentor-${u.id}`} value={mentor} onChange={(e) => setMentor(e.target.value)}>
              {mentors.length === 0 && <option value="">Наставник отдела</option>}
              {mentors.map((x) => (
                <option key={x.id} value={x.id}>{x.name}</option>
              ))}
            </select>
          </>
        ) : (
          <span className="muted small">не нужен</span>
        )}
      </td>
      <td>
        <button className="btn small" disabled={m.isPending} onClick={() => m.mutate()}>Принять в отдел</button>
      </td>
    </tr>
  );
}

const adminRoles: { value: string; label: string }[] = [
  ...assignRoles,
  { value: 'lead', label: 'Руководитель' },
  { value: 'hrbp', label: 'HR BP' },
];

/** Администратор: новые сотрудники — в любой отдел, у всех сотрудников меняет роль, отдел, наставника и пароль. */
export function AdminDepartment({ d }: { d: Dept }) {
  const [filter, setFilter] = useState('');
  const f = filter.trim().toLowerCase();
  const members = [...d.members]
    .filter((u) => !f || `${u.name} ${u.email ?? ''} ${u.teamName ?? ''} ${roleName[u.role]}`.toLowerCase().includes(f))
    .sort((a, b) => (a.teamName ?? '').localeCompare(b.teamName ?? '') || a.name.localeCompare(b.name));
  return (
    <section className="department admin-department" aria-label="Сотрудники">
      <h3 className="section-title">Новые сотрудники</h3>
      {d.newcomers.length === 0 ? (
        <p className="muted small">Нет новых регистраций.</p>
      ) : (
        <table className="compact-table">
          <thead>
            <tr>
              <th scope="col">Сотрудник</th>
              <th scope="col">Отдел</th>
              <th scope="col">Роль</th>
              <th scope="col">Наставник</th>
              <th scope="col"><span className="sr-only">Действие</span></th>
            </tr>
          </thead>
          <tbody>
            {d.newcomers.map((u) => (
              <UserEditRow key={u.id} u={u} teams={d.teams} mentors={d.mentors} newcomer />
            ))}
          </tbody>
        </table>
      )}
      <div className="row">
        <h3 className="section-title">Все сотрудники</h3>
        <label className="sr-only" htmlFor="user-filter">Найти сотрудника</label>
        <input id="user-filter" className="user-filter" value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Найти: имя, почта, отдел, роль" />
      </div>
      <p className="muted small">
        Роль «Руководитель» делает сотрудника начальником отдела: сотрудники и ждущие решения заявки отдела переходят к нему. В отделе один
        руководитель — прежнему сначала назначьте другую роль.
      </p>
      <table className="compact-table">
        <thead>
          <tr>
            <th scope="col">Сотрудник</th>
            <th scope="col">Отдел</th>
            <th scope="col">Роль</th>
            <th scope="col">Наставник</th>
            <th scope="col"><span className="sr-only">Действия</span></th>
          </tr>
        </thead>
        <tbody>
          {members.map((u) => (
            <UserEditRow key={u.id} u={u} teams={d.teams} mentors={d.mentors} />
          ))}
        </tbody>
      </table>
    </section>
  );
}

function UserEditRow({ u, teams, mentors, newcomer = false }: { u: User; teams: TeamInfo[]; mentors: User[]; newcomer?: boolean }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [team, setTeam] = useState(u.team || teams[0]?.id || '');
  const [role, setRole] = useState<string>(newcomer ? 'intern' : u.role);
  const teamMentors = mentors.filter((m) => m.team === team && m.id !== u.id);
  const [mentor, setMentor] = useState(u.mentorId ?? '');
  const [pw, setPw] = useState<string | null>(null);
  const mentorId = role === 'intern' ? (teamMentors.some((m) => m.id === mentor) ? mentor : (teamMentors[0]?.id ?? '')) : '';
  const changed = newcomer || team !== u.team || role !== u.role || (role === 'intern' && mentorId !== (u.mentorId ?? ''));
  const done = (d: Dept) => {
    qc.setQueryData(keys.department, d);
    void qc.invalidateQueries({ queryKey: keys.dashboard });
    void qc.invalidateQueries({ queryKey: keys.team });
    void qc.invalidateQueries({ queryKey: keys.rights });
  };
  const save = useMutation({
    mutationFn: () => (newcomer ? api.assign(u.id, role, mentorId, team) : api.adminSetUser(u.id, { role, team, mentorId })),
    onSuccess: (d) => {
      done(d);
      toast(`${u.name}: ${adminRoles.find((r) => r.value === role)?.label.toLowerCase()}, ${teams.find((t) => t.id === team)?.name} — сотрудник получил уведомление`);
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const reset = useMutation({
    mutationFn: () => api.adminPassword(u.id, pw ?? ''),
    onSuccess: () => {
      setPw(null);
      toast(`Пароль для ${u.name} изменён — сообщите его сотруднику лично`);
    },
    onError: (e) => toast(e.message, 'error'),
  });
  if (u.role === 'admin') return null;
  return (
    <tr>
      <td>
        <span className="person">
          <span className={`avatar role-${u.role}`} aria-hidden="true">{initials(u.name)}</span>
          <span>
            {u.name}
            <br />
            <span className="muted small">{u.email || u.login}</span>
          </span>
        </span>
      </td>
      <td>
        <label className="sr-only" htmlFor={`team-${u.id}`}>Отдел для {u.name}</label>
        <select id={`team-${u.id}`} value={team} onChange={(e) => setTeam(e.target.value)}>
          {teams.map((t) => (
            <option key={t.id} value={t.id}>{t.name}{t.leadName ? '' : ' (без руководителя)'}</option>
          ))}
        </select>
      </td>
      <td>
        <label className="sr-only" htmlFor={`arole-${u.id}`}>Роль для {u.name}</label>
        <select id={`arole-${u.id}`} value={role} onChange={(e) => setRole(e.target.value)}>
          {adminRoles.map((r) => (
            <option key={r.value} value={r.value}>{r.label}</option>
          ))}
        </select>
      </td>
      <td>
        {role === 'intern' ? (
          <>
            <label className="sr-only" htmlFor={`amentor-${u.id}`}>Наставник для {u.name}</label>
            <select id={`amentor-${u.id}`} value={mentorId} onChange={(e) => setMentor(e.target.value)}>
              {teamMentors.length === 0 && <option value="">Руководитель отдела</option>}
              {teamMentors.map((x) => (
                <option key={x.id} value={x.id}>{x.name}</option>
              ))}
            </select>
          </>
        ) : (
          <span className="muted small">—</span>
        )}
      </td>
      <td>
        <div className="row-start nowrap">
          <button className="btn small" disabled={!changed || save.isPending} onClick={() => save.mutate()}>
            {newcomer ? 'Принять' : 'Сохранить'}
          </button>
          {!newcomer &&
            (pw === null ? (
              <button className="link small" onClick={() => setPw('')}>Пароль…</button>
            ) : (
              <form
                className="pw-form"
                onSubmit={(e) => {
                  e.preventDefault();
                  reset.mutate();
                }}
              >
                <label className="sr-only" htmlFor={`pw-${u.id}`}>Новый пароль для {u.name}</label>
                <input id={`pw-${u.id}`} type="password" autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} minLength={8} required placeholder="Новый пароль" />
                <button className="btn ghost small" disabled={reset.isPending}>Задать</button>
                <button type="button" className="link small" aria-label="Отмена" onClick={() => setPw(null)}>✕</button>
              </form>
            ))}
        </div>
      </td>
    </tr>
  );
}
