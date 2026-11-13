import { useQuery } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api, type RegisterInput } from '../api/client';
import type { User } from '../api/schemas';
import { initials, roleName } from '../lib/format';

const hints: Record<string, string> = {
  anna: '4-й день стажировки, команда «Поиск объявлений». Основной сценарий.',
  ivan: 'Стажёр другой команды — для проверки разграничения прав.',
  oleg: 'Наставник Анны и владелец репозиториев search-api и search-indexer: вопросы стажёров, согласование доступов к Stash.',
  maria: 'Руководитель Анны: отгулы, доступы к Confluence кластера и Redash, метрики пользы.',
  pavel: 'Дежурный DevOps: согласует доступы к тестовому Kubernetes.',
  elena: 'HR BP команды: прогресс стажёров и метрики.',
  admin: 'Администратор сервиса: выше руководителей — роли и отделы всех сотрудников, любые согласования, флаги возможностей, журнал действий.',
};

export function Login({ onDone, initialMode = 'login' }: { onDone: (u: User) => void; initialMode?: 'login' | 'register' }) {
  const [mode, setMode] = useState(initialMode);
  if (mode === 'register') return <Register onDone={onDone} onBack={() => setMode('login')} />;
  return <SignIn onDone={onDone} onRegister={() => setMode('register')} />;
}

function Intro() {
  return (
    <section className="login-intro">
      <div className="logo-note" aria-hidden="true">
        <span>AI</span>
      </div>
      <h1>AI-buddy</h1>
      <p className="lead">Помощник стажёра в первые недели: отвечает на рабочие вопросы, ведёт по плану адаптации и оформляет заявки через корпоративные сервисы.</p>
      <p className="muted small">Прототип для кейса Hack&amp;Change 2026. Все данные и инструкции синтетические.</p>
    </section>
  );
}

function SignIn({ onDone, onRegister }: { onDone: (u: User) => void; onRegister: () => void }) {
  const demo = useQuery({ queryKey: ['demo-users'], queryFn: api.demoUsers });
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');

  async function enter(l: string, p: string) {
    setBusy(l);
    setError('');
    try {
      const r = await api.login(l, p);
      onDone(r.user);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось войти');
    } finally {
      setBusy('');
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    void enter(login, password);
  }

  const users = (demo.data?.users ?? []).filter((u) => hints[u.login]);
  return (
    <main className="login">
      <Intro />
      <section className="login-panel" aria-labelledby="demo-title">
        <h2 id="demo-title">Войти тестовой учётной записью</h2>
        {demo.isError && <p className="error">Не удалось загрузить тестовые учётные записи. Войди вручную ниже.</p>}
        <ul className="accounts">
          {users.map((u) => (
            <li key={u.id}>
              <button className="account" disabled={!!busy} onClick={() => void enter(u.login, demo.data!.password)}>
                <span className={`avatar role-${u.role}`} aria-hidden="true">{initials(u.name)}</span>
                <span className="account-text">
                  <strong>{u.name}</strong>
                  <span className="account-role">{roleName[u.role]}</span>
                  <span className="muted">{hints[u.login]}</span>
                </span>
                {busy === u.login && <span className="spinner" aria-label="Входим" />}
              </button>
            </li>
          ))}
        </ul>
        <form className="manual" onSubmit={submit}>
          <label>
            Логин или почта
            <input value={login} onChange={(e) => setLogin(e.target.value)} autoComplete="username" required />
          </label>
          <label>
            Пароль
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
          </label>
          <button className="btn" type="submit" disabled={!!busy}>Войти</button>
        </form>
        {error && <p className="error" role="alert">{error}</p>}
        <p className="register-cta">
          Нет учётной записи?{' '}
          <button className="link" onClick={onRegister}>Зарегистрироваться</button>
        </p>
      </section>
    </main>
  );
}

export function Register({ onDone, onBack }: { onDone: (u: User) => void; onBack: () => void }) {
  const [form, setForm] = useState<RegisterInput>({ name: '', email: '', password: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (patch: Partial<RegisterInput>) => setForm({ ...form, ...patch });
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const r = await api.register(form);
      onDone(r.user);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось зарегистрироваться');
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className="login">
      <Intro />
      <section className="login-panel" aria-labelledby="reg-title">
        <h2 id="reg-title">Регистрация</h2>
        <p className="muted small">Отдел, роль и наставника назначит руководитель — после регистрации он получит уведомление.</p>
        <form className="register" onSubmit={submit}>
          <label className="field">
            ФИО
            <input value={form.name} onChange={(e) => set({ name: e.target.value })} autoComplete="name" placeholder="Сидоров Пётр Андреевич" required />
          </label>
          <label className="field">
            Рабочая почта
            <input type="email" value={form.email} onChange={(e) => set({ email: e.target.value })} autoComplete="email" placeholder="psidorov@example.com" required />
          </label>
          <label className="field">
            Пароль
            <input type="password" value={form.password} onChange={(e) => set({ password: e.target.value })} autoComplete="new-password" minLength={8} required />
            <span className="small">Не короче 8 символов, буквы и цифры</span>
          </label>
          <button className="btn" disabled={busy}>{busy ? 'Создаю учётную запись…' : 'Зарегистрироваться'}</button>
          {error && <p className="error" role="alert">{error}</p>}
        </form>
        <p className="register-cta">
          Уже есть учётная запись? <button className="link" onClick={onBack}>Войти</button>
        </p>
      </section>
    </main>
  );
}
