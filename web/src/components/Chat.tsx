import { useEffect, useRef, type FormEvent, type KeyboardEvent } from 'react';
import type { Action, Message, User } from '../api/schemas';
import { byCreated, useChat, useConfig, useFeedback, useSend } from '../hooks/queries';
import { firstName, time } from '../lib/format';
import { Markdown } from '../lib/markdown';
import { useUi } from '../store/ui';
import { ActionCard } from './ActionCard';

const starters: Record<User['role'], string[]> = {
  intern: ['Что мне делать дальше?', 'Какие доступы мне нужны?', 'Мне нужен отгул — как оформить и стоит ли писать руководителю?', 'Кто мой наставник?'],
  mentor: ['Как дела у моих стажёров?', 'Что такое Zero Bug Policy?', 'Как подготовить стажёра к экватору?'],
  lead: ['Кто сейчас у нас стажируется?', 'Что такое Zero Bug Policy?', 'Как стажёру получить доступ к Redash?'],
  hrbp: ['Кто сейчас стажируется?', 'Как проходит экваторная встреча?'],
  engineer: ['Какие доступы нужны стажёру?'],
  newcomer: [],
};

export function Chat({ user }: { user: User }) {
  const chat = useChat();
  const send = useSend();
  const cfg = useConfig();
  const { draft, setDraft } = useUi();
  const endRef = useRef<HTMLDivElement>(null);
  const messages = byCreated(chat.data?.messages ?? []);
  const actions = new Map((chat.data?.actions ?? []).map((a) => [a.id, a]));
  const pending = messages.some((m) => m.status === 'pending');
  const last = messages[messages.length - 1];

  useEffect(() => {
    endRef.current?.scrollIntoView?.({ block: 'end' });
  }, [messages.length, last?.status, last?.trace.length]);

  function submit(text: string) {
    const t = text.trim();
    if (!t || send.isPending) return;
    setDraft('');
    send.mutate(t);
  }
  function onSubmit(e: FormEvent) {
    e.preventDefault();
    submit(draft);
  }
  function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit(draft);
    }
  }

  return (
    <section className="chat" aria-label="Диалог с AI-buddy">
      <div className="messages" role="log" aria-live="polite">
        {chat.isPending && <p className="muted center">Загружаю историю…</p>}
        {chat.isError && <p className="error center">Не удалось загрузить историю. Обнови страницу.</p>}
        {chat.isSuccess && messages.length === 0 && (
          <div className="empty">
            <h2>Привет, {firstName(user.name)}! Спроси о команде, процессах или инструментах.</h2>
            <p className="muted">Я отвечаю по инструкциям команды и показываю источник. Заявки на отгул и доступы готовлю сам — ты только проверяешь и отправляешь.</p>
          </div>
        )}
        {messages.map((m) => (
          <Bubble key={m.id} m={m} action={m.actionId ? actions.get(m.actionId) : undefined} onSuggest={submit} isLast={m === last} />
        ))}
        <div ref={endRef} />
      </div>
      {messages.length === 0 && (
        <div className="chips" aria-label="Примеры вопросов">
          {starters[user.role].map((s) => (
            <button key={s} className="chip" onClick={() => submit(s)}>{s}</button>
          ))}
        </div>
      )}
      <form className="composer" onSubmit={onSubmit}>
        <label className="sr-only" htmlFor="composer">Сообщение</label>
        <textarea
          id="composer"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKey}
          placeholder="Например: запроси доступ к Stash"
          rows={1}
          maxLength={2000}
        />
        <button className="btn send" type="submit" disabled={!draft.trim() || send.isPending}>
          {pending ? 'Отвечаю…' : 'Отправить'}
        </button>
      </form>
      {cfg.data && (
        <p className="mode muted small">
          {cfg.data.mode === 'llm' ? 'Режим: языковая модель с инструментами' : 'Режим: офлайн-агент без внешней модели'}
        </p>
      )}
    </section>
  );
}

export function Bubble({ m, action, onSuggest, isLast }: { m: Message; action?: Action; onSuggest: (t: string) => void; isLast: boolean }) {
  const feedback = useFeedback();
  if (m.author === 'user') {
    return (
      <div className="bubble user">
        <p>{m.text}</p>
        <span className="when">{time(m.createdAt)}</span>
      </div>
    );
  }
  if (m.status === 'pending') {
    return (
      <div className="bubble assistant pending" aria-busy="true">
        <ol className="trace">
          {m.trace.map((t, i) => (
            <li key={i} className={i === m.trace.length - 1 ? 'now' : 'past'}>{t.label}</li>
          ))}
        </ol>
      </div>
    );
  }
  return (
    <div className={`bubble assistant ${m.status === 'error' ? 'failed' : ''} ${m.mode === 'mentor' ? 'from-mentor' : ''} ${m.mode === 'proactive' ? 'proactive' : ''}`}>
      {m.mode === 'proactive' && <p className="muted small">Напоминание AI-buddy</p>}
      <div className="md">
        <Markdown text={m.text} />
      </div>
      {m.sources.length > 0 && (
        <div className="sources">
          <span className="muted small">Источник:</span>
          {m.sources.map((s) => (
            <SourceLink key={s.docId + (s.section ?? '')} docId={s.docId} label={s.section && !s.title.includes(s.section) ? `${s.title} — ${s.section}` : s.title} />
          ))}
        </div>
      )}
      {action && <ActionCard action={action} />}
      {m.quizId && <QuizButton id={m.quizId} />}
      {isLast && m.suggestions.length > 0 && (
        <div className="chips inline">
          {m.suggestions.map((s) => (
            <button key={s} className="chip" onClick={() => onSuggest(s)}>{s}</button>
          ))}
        </div>
      )}
      <div className="bubble-foot">
        <span className="when">{time(m.createdAt)}</span>
        {m.mode !== 'mentor' && (
          <span className="rate" role="group" aria-label="Оценить ответ">
            <button className={m.feedback === true ? 'on' : ''} onClick={() => feedback.mutate({ id: m.id, helpful: true })} aria-pressed={m.feedback === true}>
              Помогло
            </button>
            <button className={m.feedback === false ? 'on' : ''} onClick={() => feedback.mutate({ id: m.id, helpful: false })} aria-pressed={m.feedback === false}>
              Не помогло
            </button>
          </span>
        )}
      </div>
    </div>
  );
}

function SourceLink({ docId, label }: { docId: string; label: string }) {
  const go = useUi((s) => s.go);
  return (
    <button className="source" onClick={() => go('knowledge', { docId })}>
      {label}
    </button>
  );
}

function QuizButton({ id }: { id: string }) {
  const go = useUi((s) => s.go);
  return (
    <button className="btn small quiz-cta" onClick={() => go('quizzes', { quizId: id })}>
      Пройти тест
    </button>
  );
}
