import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { keys } from '../hooks/queries';
import { Markdown } from '../lib/markdown';
import { useUi } from '../store/ui';

export function Knowledge() {
  const { docId, go } = useUi();
  const docs = useQuery({ queryKey: keys.docs, queryFn: api.docs });
  const doc = useQuery({ queryKey: keys.doc(docId ?? ''), queryFn: () => api.doc(docId!), enabled: !!docId });
  return (
    <section className="kb">
      <nav className="kb-list" aria-label="Инструкции">
        <p className="muted small">Показаны инструкции, доступные твоей роли и команде.</p>
        <ul>
          {(docs.data?.docs ?? []).map((d) => (
            <li key={d.id}>
              <button className={d.id === docId ? 'active' : ''} onClick={() => go('knowledge', { docId: d.id })}>{d.title}</button>
            </li>
          ))}
        </ul>
      </nav>
      <article className="kb-doc">
        {!docId && <p className="muted">Выбери инструкцию слева или спроси AI-buddy в чате — он найдёт нужный раздел сам.</p>}
        {doc.isError && <p className="error">Инструкция недоступна.</p>}
        {doc.data && (
          <>
            <p className="synthetic">Тестовая инструкция прототипа — не регламент Авито</p>
            <h2>{doc.data.title}</h2>
            {doc.data.sections.map((s) => (
              <section key={s.heading}>
                <h3>{s.heading}</h3>
                <div className="md">
                  <Markdown text={s.text} />
                </div>
              </section>
            ))}
          </>
        )}
      </article>
    </section>
  );
}
