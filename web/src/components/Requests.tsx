import { useChat } from '../hooks/queries';
import { useUi } from '../store/ui';
import { ActionCard } from './ActionCard';

export function Requests() {
  const chat = useChat();
  const go = useUi((s) => s.go);
  const actions = (chat.data?.actions ?? []).filter((a) => a.status !== 'cancelled');
  if (chat.isPending) return <p className="muted">Загружаю заявки…</p>;
  if (actions.length === 0)
    return (
      <div className="empty">
        <h2>Заявок пока нет</h2>
        <p className="muted">Напиши в чат, например «запроси доступ к Stash» или «отгул в пятницу», — я подготовлю заявку и покажу её здесь.</p>
        <button className="btn" onClick={() => go('chat')}>Перейти в чат</button>
      </div>
    );
  return (
    <section className="board">
      {actions.map((a) => (
        <ActionCard key={a.id} action={a} />
      ))}
    </section>
  );
}
