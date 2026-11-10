// Поток обновлений с сервера (SSE) с автоматическим переподключением.
import { Update } from './schemas';

export type UpdateHandler = (u: { type: string; data?: unknown }) => void;

export interface EventSourceLike {
  onmessage: ((e: { data: string }) => void) | null;
  onerror: ((e: unknown) => void) | null;
  onopen: ((e: unknown) => void) | null;
  close(): void;
}

export function subscribe(
  onUpdate: UpdateHandler,
  onStatus: (online: boolean) => void,
  factory: (url: string) => EventSourceLike = (url) => new EventSource(url) as unknown as EventSourceLike,
): () => void {
  const es = factory('/api/events');
  es.onopen = () => onStatus(true);
  es.onerror = () => onStatus(false);
  es.onmessage = (e) => {
    try {
      const parsed = Update.safeParse(JSON.parse(e.data));
      if (parsed.success) onUpdate(parsed.data);
    } catch {
      /* битое сообщение игнорируем */
    }
  };
  return () => es.close();
}
