import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { User } from '../api/schemas';
import { useLiveUpdates, useNotifications, keys, usePerms } from '../hooks/queries';
import { initials, relative, roleName } from '../lib/format';
import { useUi, type View } from '../store/ui';
import { useInstallPrompt, viewFromSearch } from '../pwa';
import { Approvals } from './Approvals';
import { Chat } from './Chat';
import { Inbox } from './Inbox';
import { Insights } from './Insights';
import { Knowledge } from './Knowledge';
import { PlanView } from './PlanView';
import { Programs } from './Programs';
import { Quizzes } from './Quizzes';
import { Requests } from './Requests';
import { Rights } from './Rights';
import { Team } from './Team';
import { TodayPanel } from './TodayPanel';

type NavItem = { view: View; label: string };

const navByRole: Record<User['role'], NavItem[]> = {
  intern: [
    { view: 'chat', label: 'Чат' },
    { view: 'plan', label: 'Мой план' },
    { view: 'quizzes', label: 'Тесты' },
    { view: 'requests', label: 'Заявки' },
    { view: 'knowledge', label: 'Инструкции' },
  ],
  mentor: [
    { view: 'inbox', label: 'Вопросы стажёров' },
    { view: 'approvals', label: 'Согласования' },
    { view: 'team', label: 'Стажёры' },
    { view: 'programs', label: 'Шаблоны обучения' },
    { view: 'insights', label: 'Польза' },
    { view: 'chat', label: 'Чат' },
    { view: 'knowledge', label: 'Инструкции' },
    { view: 'quizzes', label: 'Тесты' },
  ],
  lead: [
    { view: 'approvals', label: 'Согласования' },
    { view: 'team', label: 'Стажёры' },
    { view: 'programs', label: 'Шаблоны обучения' },
    { view: 'rights', label: 'Отдел и права' },
    { view: 'insights', label: 'Польза' },
    { view: 'chat', label: 'Чат' },
    { view: 'knowledge', label: 'Инструкции' },
    { view: 'quizzes', label: 'Тесты' },
  ],
  engineer: [
    { view: 'approvals', label: 'Согласования' },
    { view: 'chat', label: 'Чат' },
    { view: 'knowledge', label: 'Инструкции' },
  ],
  hrbp: [
    { view: 'team', label: 'Стажёры' },
    { view: 'insights', label: 'Польза' },
    { view: 'chat', label: 'Чат' },
    { view: 'knowledge', label: 'Инструкции' },
    { view: 'quizzes', label: 'Тесты' },
  ],
};

/** navFor — разделы по роли и выданным правам. */
export function navFor(role: User['role'], perms: Set<string>): NavItem[] {
  const nav = [...navByRole[role]];
  if (perms.has('approvals') && !nav.some((n) => n.view === 'approvals')) nav.unshift({ view: 'approvals', label: 'Согласования' });
  if (perms.has('kb') && !nav.some((n) => n.view === 'quizzes')) nav.push({ view: 'quizzes', label: 'Тесты' });
  return nav;
}

export function Shell({ user }: { user: User }) {
  const { view, go, online, toasts, dismiss } = useUi();
  const perms = usePerms();
  const nav = navFor(user.role, perms);
  const qc = useQueryClient();
  useLiveUpdates(true);
  const first = useRef(true);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      const fromShortcut = viewFromSearch(window.location.search);
      if (fromShortcut && nav.some((n) => n.view === fromShortcut)) go(fromShortcut);
      else if (user.role !== 'intern' && view === 'chat') go(nav[0]!.view);
    }
  }, [user.role, view, go, nav]);

  async function logout() {
    await api.logout().catch(() => undefined);
    qc.clear();
    qc.setQueryData(keys.me, undefined);
    window.location.reload();
  }

  return (
    <div className={`shell view-${view}`}>
      <nav className="rail" aria-label="Разделы">
        <div className="brand">
          <span className="brand-note" aria-hidden="true">AI</span>
          <span>buddy</span>
        </div>
        <ul>
          {nav.map((n) => (
            <li key={n.view}>
              <button className={view === n.view ? 'active' : ''} aria-current={view === n.view ? 'page' : undefined} onClick={() => go(n.view)}>
                {n.label}
              </button>
            </li>
          ))}
        </ul>
        <div className="me">
          <span className={`avatar role-${user.role}`} aria-hidden="true">{initials(user.name)}</span>
          <span className="me-text">
            <strong>{user.name}</strong>
            <span className="muted">{roleName[user.role]}, {user.teamName}</span>
          </span>
          <button className="link" onClick={() => void logout()}>Выйти</button>
        </div>
      </nav>
      <div className="main">
        <header className="topbar">
          <h1 className="view-title">{nav.find((n) => n.view === view)?.label ?? 'План стажёра'}</h1>
          {!online && <span className="offline" role="status">Нет живого соединения — обновляю данные опросом</span>}
          <InstallButton />
          <Bell />
        </header>
        <div className="content">
          {view === 'chat' && <Chat user={user} />}
          {view === 'plan' && <PlanView />}
          {view === 'requests' && <Requests />}
          {view === 'knowledge' && <Knowledge />}
          {view === 'quizzes' && <Quizzes role={user.role} />}
          {view === 'programs' && <Programs />}
          {view === 'rights' && <Rights />}
          {view === 'inbox' && <Inbox />}
          {view === 'approvals' && <Approvals />}
          {view === 'team' && <Team />}
          {view === 'insights' && <Insights />}
        </div>
      </div>
      {user.role === 'intern' && view === 'chat' && <TodayPanel />}
      <div className="toasts" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`toast ${t.tone}`} onClick={() => dismiss(t.id)}>
            {t.text}
          </div>
        ))}
      </div>
    </div>
  );
}

function InstallButton() {
  const { canInstall, install } = useInstallPrompt();
  if (!canInstall) return null;
  return (
    <button className="btn ghost small install" onClick={() => void install()}>
      Установить приложение
    </button>
  );
}

function Bell() {
  const n = useNotifications();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const list = n.data?.notifications ?? [];
  const unread = list.filter((x) => !x.read).length;
  function toggle() {
    setOpen(!open);
    if (!open && unread > 0) {
      void api.readNotifications().then(() =>
        qc.setQueryData(keys.notifications, { notifications: list.map((x) => ({ ...x, read: true })) }),
      );
    }
  }
  return (
    <div className="bell">
      <button className="bell-btn" onClick={toggle} aria-expanded={open} aria-label={`Уведомления: ${unread} новых`}>
        <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true">
          <path d="M12 3a6 6 0 0 0-6 6v3.5L4.5 16h15L18 12.5V9a6 6 0 0 0-6-6Zm-2 15a2 2 0 0 0 4 0" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round" />
        </svg>
        {unread > 0 && <span className="badge">{unread}</span>}
      </button>
      {open && (
        <div className="popover" role="dialog" aria-label="Уведомления">
          {list.length === 0 && <p className="muted">Пока тихо. Здесь появятся статусы заявок и ответы наставника.</p>}
          <ul>
            {list.slice(0, 12).map((x) => (
              <li key={x.id} className={x.read ? '' : 'unread'}>
                <span>{x.text}</span>
                <span className="muted small">{relative(x.createdAt)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
