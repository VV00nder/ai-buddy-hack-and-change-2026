import { QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { ApiError } from './api/client';
import { Login } from './components/Login';
import { Shell } from './components/Shell';
import { Waiting } from './components/Waiting';
import { keys, makeQueryClient, useMe } from './hooks/queries';

export function App({ client }: { client?: ReturnType<typeof makeQueryClient> }) {
  const [qc] = useState(() => client ?? makeQueryClient());
  return (
    <QueryClientProvider client={qc}>
      <Root />
    </QueryClientProvider>
  );
}

export function Root() {
  const me = useMe();
  const qc = useQueryClient();
  const offline = typeof navigator !== 'undefined' && navigator.onLine === false;
  useEffect(() => {
    // связь вернулась — перезагружаем данные
    const onOnline = () => void qc.invalidateQueries();
    window.addEventListener('online', onOnline);
    return () => window.removeEventListener('online', onOnline);
  }, [qc]);
  if (me.isPending) return <div className="boot" aria-busy="true">AI-buddy загружается…</div>;
  if (me.error instanceof ApiError && me.error.status === 401) {
    return (
      <Login
        onDone={(user) => {
          qc.setQueryData(keys.me, { user, perms: [] });
          void qc.invalidateQueries({ queryKey: keys.me });
        }}
      />
    );
  }
  if (me.error || !me.data) {
    return (
      <div className="boot">
        <p>{offline ? 'Нет подключения к сети. AI-buddy обновится сам, когда связь появится.' : 'Сервер AI-buddy не отвечает.'}</p>
        <button className="btn" onClick={() => void me.refetch()}>Повторить</button>
      </div>
    );
  }
  if (me.data.user.role === 'newcomer') return <Waiting user={me.data.user} />;
  return <Shell user={me.data.user} />;
}
