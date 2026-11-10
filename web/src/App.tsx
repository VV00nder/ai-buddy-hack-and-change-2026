import { QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { ApiError } from './api/client';
import { Login } from './components/Login';
import { Shell } from './components/Shell';
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
  if (me.isPending) return <div className="boot" aria-busy="true">AI-buddy загружается…</div>;
  if (me.error instanceof ApiError && me.error.status === 401) {
    return <Login onDone={(user) => qc.setQueryData(keys.me, { user })} />;
  }
  if (me.error || !me.data) {
    return (
      <div className="boot">
        <p>Сервер AI-buddy не отвечает.</p>
        <button className="btn" onClick={() => void me.refetch()}>Повторить</button>
      </div>
    );
  }
  return <Shell user={me.data.user} />;
}
