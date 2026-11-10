import { useQuery } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { User } from '../api/schemas';
import { initials, roleName } from '../lib/format';

const hints: Record<string, string> = {
  anna: '4-й день стажировки, команда «Поиск объявлений». Основной сценарий.',
  ivan: 'Стажёр другой команды — для проверки разграничения прав.',
  oleg: 'Наставник Анны: вопросы стажёров и их прогресс.',
  maria: 'Руководитель Анны: согласования и метрики пользы.',
  elena: 'HR BP команды: прогресс стажёров и метрики.',
};

export function Login({ onDone }: { onDone: (u: User) => void }) {
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
      <section className="login-intro">
        <div className="logo-note" aria-hidden="true">
          <span>AI</span>
        </div>
        <h1>AI-buddy</h1>
        <p className="lead">Помощник стажёра в первые недели: отвечает на рабочие вопросы, ведёт по плану адаптации и оформляет заявки через корпоративные сервисы.</p>
        <p className="muted small">Прототип для кейса Hack&amp;Change 2026. Все данные и инструкции синтетические.</p>
      </section>
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
            Логин
            <input value={login} onChange={(e) => setLogin(e.target.value)} autoComplete="username" required />
          </label>
          <label>
            Пароль
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
          </label>
          <button className="btn" type="submit" disabled={!!busy}>Войти</button>
        </form>
        {error && <p className="error" role="alert">{error}</p>}
      </section>
    </main>
  );
}
