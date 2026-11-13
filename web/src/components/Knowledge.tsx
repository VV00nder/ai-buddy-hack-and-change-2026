import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDeferredValue, useMemo, useState, type FormEvent } from 'react';
import { api, type DocInput } from '../api/client';
import type { Doc, DocSummary } from '../api/schemas';
import { keys, usePerms } from '../hooks/queries';
import { plural } from '../lib/format';
import { Markdown } from '../lib/markdown';
import { useUi } from '../store/ui';

/** Узел дерева папок. */
export interface FolderNode {
  name: string;
  path: string;
  children: FolderNode[];
  docs: DocSummary[];
}

/** buildTree раскладывает инструкции по папкам (вложенность через «/»). */
export function buildTree(folders: string[], docs: DocSummary[]): FolderNode {
  const root: FolderNode = { name: '', path: '', children: [], docs: [] };
  const byPath = new Map<string, FolderNode>([['', root]]);
  const ensure = (path: string): FolderNode => {
    const found = byPath.get(path);
    if (found) return found;
    const i = path.lastIndexOf('/');
    const parent = ensure(i === -1 ? '' : path.slice(0, i));
    const node: FolderNode = { name: path.slice(i + 1), path, children: [], docs: [] };
    parent.children.push(node);
    byPath.set(path, node);
    return node;
  };
  for (const f of folders) ensure(f);
  for (const d of docs) ensure(d.folder).docs.push(d);
  const sort = (n: FolderNode) => {
    n.children.sort((a, b) => a.name.localeCompare(b.name, 'ru'));
    n.docs.sort((a, b) => a.title.localeCompare(b.title, 'ru'));
    n.children.forEach(sort);
  };
  sort(root);
  return root;
}

export const audienceNames: Record<string, string> = {
  all: 'Все',
  intern: 'Стажёры',
  mentor: 'Наставники',
  lead: 'Руководители',
  hrbp: 'HR BP',
  engineer: 'Инженеры',
};

export function Knowledge() {
  const { docId, go } = useUi();
  const perms = usePerms();
  const canEdit = perms.has('kb');
  const [q, setQ] = useState('');
  const query = useDeferredValue(q);
  const [editing, setEditing] = useState<'new' | 'edit' | null>(null);
  const [newFolder, setNewFolder] = useState<string | null>(null);
  const docs = useQuery({ queryKey: keys.docs(query), queryFn: () => api.docs(query) });
  const all = useQuery({ queryKey: keys.docs(''), queryFn: () => api.docs('') });
  const doc = useQuery({ queryKey: keys.doc(docId ?? ''), queryFn: () => api.doc(docId!), enabled: !!docId });
  const tree = useMemo(() => buildTree(all.data?.folders ?? [], all.data?.docs ?? []), [all.data]);
  const folders = all.data?.folders ?? [];
  const searching = query.trim().length > 0;
  const open = (id: string) => {
    setEditing(null);
    go('knowledge', { docId: id });
  };

  return (
    <section className="kb">
      <nav className="kb-list" aria-label="Инструкции">
        <label className="kb-search">
          <span className="sr-only">Поиск по инструкциям</span>
          <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Поиск по инструкциям" />
        </label>
        {canEdit && (
          <div className="kb-tools">
            <button className="btn small" onClick={() => setEditing('new')}>Новая инструкция</button>
            <button className="btn ghost small" onClick={() => setNewFolder(newFolder === null ? '' : null)}>Новая папка</button>
          </div>
        )}
        {newFolder !== null && <FolderForm value={newFolder} onChange={setNewFolder} onDone={() => setNewFolder(null)} />}
        {searching ? (
          <SearchResults docs={docs.data?.docs ?? []} pending={docs.isFetching} docId={docId} onOpen={open} />
        ) : (
          <>
            <p className="muted small">Показаны инструкции, доступные твоей роли и команде.</p>
            <Tree node={tree} docId={docId} onOpen={open} canEdit={canEdit} />
          </>
        )}
      </nav>
      <article className="kb-doc">
        {editing === 'new' && <DocEditor folders={folders} onDone={(id) => (id ? open(id) : setEditing(null))} />}
        {editing === 'edit' && doc.data && <DocEditor doc={doc.data} folders={folders} onDone={() => setEditing(null)} />}
        {!editing && (
          <>
            {!docId && <p className="muted">Выбери инструкцию слева или спроси AI-buddy в чате — он найдёт нужный раздел сам.</p>}
            {docId && doc.isError && <p className="error">Инструкция недоступна.</p>}
            {doc.data && <DocView doc={doc.data} onEdit={() => setEditing('edit')} />}
          </>
        )}
      </article>
    </section>
  );
}

function SearchResults({ docs, pending, docId, onOpen }: { docs: DocSummary[]; pending: boolean; docId: string | null; onOpen: (id: string) => void }) {
  if (docs.length === 0) return <p className="muted small kb-empty">{pending ? 'Ищу…' : 'Ничего не нашлось. Попробуй другие слова или спроси AI-buddy в чате.'}</p>;
  return (
    <ul aria-label="Результаты поиска">
      {docs.map((d) => (
        <li key={d.id}>
          <button className={d.id === docId ? 'active' : ''} onClick={() => onOpen(d.id)}>
            {d.title}
            {d.folder && <span className="muted small kb-path">{d.folder}</span>}
          </button>
        </li>
      ))}
    </ul>
  );
}

function Tree({ node, docId, onOpen, canEdit }: { node: FolderNode; docId: string | null; onOpen: (id: string) => void; canEdit: boolean }) {
  return (
    <ul className="tree">
      {node.children.map((c) => (
        <li key={c.path}>
          <FolderItem node={c} docId={docId} onOpen={onOpen} canEdit={canEdit} />
        </li>
      ))}
      {node.docs.map((d) => (
        <li key={d.id}>
          <button className={d.id === docId ? 'active' : ''} aria-current={d.id === docId ? 'page' : undefined} onClick={() => onOpen(d.id)}>
            {d.title}
          </button>
        </li>
      ))}
    </ul>
  );
}

function countDocs(n: FolderNode): number {
  return n.docs.length + n.children.reduce((s, c) => s + countDocs(c), 0);
}

function contains(n: FolderNode, id: string | null): boolean {
  return !!id && (n.docs.some((d) => d.id === id) || n.children.some((c) => contains(c, id)));
}

function FolderItem({ node, docId, onOpen, canEdit }: { node: FolderNode; docId: string | null; onOpen: (id: string) => void; canEdit: boolean }) {
  const [open, setOpen] = useState(() => contains(node, docId) || !node.path.includes('/'));
  const n = countDocs(node);
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const del = useMutation({
    mutationFn: () => api.deleteFolder(node.path),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['docs'] }),
    onError: (e) => toast(e.message, 'error'),
  });
  return (
    <div className="folder">
      <div className="folder-row">
        <button className="folder-btn" aria-expanded={open} onClick={() => setOpen(!open)}>
          <span className={`chev ${open ? 'open' : ''}`} aria-hidden="true" />
          <span className="folder-name">{node.name}</span>
          <span className="muted small">{n}</span>
        </button>
        {canEdit && n === 0 && (
          <button className="link small" onClick={() => del.mutate()} aria-label={`Удалить пустую папку «${node.path}»`}>
            удалить
          </button>
        )}
      </div>
      {open && <Tree node={node} docId={docId} onOpen={onOpen} canEdit={canEdit} />}
    </div>
  );
}

export function FolderForm({ value, onChange, onDone }: { value: string; onChange: (v: string) => void; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const m = useMutation({
    mutationFn: () => api.createFolder(value),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['docs'] });
      toast('Папка создана');
      onDone();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  function submit(e: FormEvent) {
    e.preventDefault();
    m.mutate();
  }
  return (
    <form className="folder-form" onSubmit={submit}>
      <label className="field">
        Путь папки
        <input autoFocus value={value} onChange={(e) => onChange(e.target.value)} placeholder="Процессы/Релизы" />
      </label>
      <span className="muted small">Вложенность — через «/», не глубже 4 уровней.</span>
      <div className="row-start">
        <button className="btn small" disabled={!value.trim() || m.isPending}>Создать</button>
        <button type="button" className="link small" onClick={onDone}>Отмена</button>
      </div>
    </form>
  );
}

export function DocView({ doc, onEdit }: { doc: Doc; onEdit: () => void }) {
  const go = useUi((s) => s.go);
  const quizzes = useQuery({ queryKey: keys.quizzes, queryFn: api.quizzes });
  const related = (quizzes.data?.quizzes ?? []).filter((q) => q.docIds.includes(doc.id));
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const del = useMutation({
    mutationFn: () => api.deleteDoc(doc.id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['docs'] });
      toast('Инструкция удалена');
      go('knowledge');
    },
    onError: (e) => toast(e.message, 'error'),
  });
  return (
    <>
      <div className="doc-meta">
        {doc.folder && <span className="muted small crumbs">{doc.folder.split('/').join(' / ')}</span>}
        {doc.custom ? (
          <span className="tag team-tag">Инструкция команды{doc.updatedBy ? `, ${doc.updatedBy}` : ''}</span>
        ) : (
          <p className="synthetic">Тестовая инструкция прототипа — не регламент Авито</p>
        )}
      </div>
      <h2>{doc.title}</h2>
      {doc.editable && (
        <div className="row-start">
          <button className="btn ghost small" onClick={onEdit}>Редактировать</button>
          <button
            className="link small danger"
            disabled={del.isPending}
            onClick={() => {
              if (typeof window === 'undefined' || window.confirm('Удалить инструкцию? Это действие нельзя отменить.')) del.mutate();
            }}
          >
            Удалить
          </button>
        </div>
      )}
      {doc.sections.map((s) => (
        <section key={s.heading}>
          <h3>{s.heading}</h3>
          <div className="md">
            <Markdown text={s.text} />
          </div>
        </section>
      ))}
      {related.length > 0 && (
        <aside className="check-yourself" aria-label="Проверь себя">
          <h3>Проверь себя</h3>
          {related.map((q) => (
            <div key={q.id} className="row">
              <span>
                Тест «{q.title}», {q.questions} {plural(q.questions, 'вопрос', 'вопроса', 'вопросов')}
                {q.passed && <span className="tag pass-tag">сдан, {q.best}%</span>}
              </span>
              <button className="btn small" onClick={() => go('quizzes', { quizId: q.id })}>
                {q.passed ? 'Пройти ещё раз' : 'Пройти тест'}
              </button>
            </div>
          ))}
        </aside>
      )}
    </>
  );
}

export function DocEditor({ doc, folders, onDone }: { doc?: Doc; folders: string[]; onDone: (id?: string) => void }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [form, setForm] = useState<DocInput>(() => ({
    title: doc?.title ?? '',
    folder: doc?.folder ?? '',
    audience: doc?.audience.length ? doc.audience : ['all'],
    tags: doc?.tags ?? [],
    body: doc?.body ?? '',
  }));
  const [tags, setTags] = useState((doc?.tags ?? []).join(', '));
  const [preview, setPreview] = useState(false);
  const m = useMutation({
    mutationFn: () => api.saveDoc(doc?.id ?? null, { ...form, tags: tags.split(',').map((t) => t.trim()).filter(Boolean) }),
    onSuccess: (d) => {
      void qc.invalidateQueries({ queryKey: ['docs'] });
      qc.setQueryData(keys.doc(d.id), d);
      toast(doc ? 'Инструкция сохранена' : 'Инструкция добавлена — AI-buddy уже отвечает по ней');
      onDone(d.id);
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const toggleAud = (r: string) => {
    if (r === 'all') return setForm({ ...form, audience: ['all'] });
    const cur = form.audience.filter((x) => x !== 'all');
    const next = cur.includes(r) ? cur.filter((x) => x !== r) : [...cur, r];
    setForm({ ...form, audience: next.length ? next : ['all'] });
  };
  function submit(e: FormEvent) {
    e.preventDefault();
    m.mutate();
  }
  return (
    <form className="editor" onSubmit={submit} aria-label={doc ? 'Редактирование инструкции' : 'Новая инструкция'}>
      <h2>{doc ? 'Редактирование' : 'Новая инструкция'}</h2>
      <label className="field">
        Название
        <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} required minLength={3} maxLength={150} />
      </label>
      <div className="editor-grid">
        <label className="field">
          Папка
          <input list="kb-folders" value={form.folder} onChange={(e) => setForm({ ...form, folder: e.target.value })} placeholder="Например: Процессы/Релизы" />
          <datalist id="kb-folders">
            {folders.map((f) => (
              <option key={f} value={f} />
            ))}
          </datalist>
        </label>
        <label className="field">
          Теги через запятую
          <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="релиз, деплой" />
        </label>
      </div>
      <fieldset className="audience">
        <legend className="small muted">Кому видна</legend>
        {Object.entries(audienceNames).map(([r, label]) => (
          <label key={r} className="checkbox">
            <input type="checkbox" checked={form.audience.includes(r)} onChange={() => toggleAud(r)} />
            {label}
          </label>
        ))}
      </fieldset>
      <div className="field">
        <div className="row">
          <label htmlFor="doc-body">Текст (markdown, разделы — строкой «## Заголовок»)</label>
          <button type="button" className="link small" onClick={() => setPreview(!preview)}>{preview ? 'Редактировать' : 'Предпросмотр'}</button>
        </div>
        {preview ? (
          <div className="md preview">
            <Markdown text={form.body} />
          </div>
        ) : (
          <textarea id="doc-body" rows={14} value={form.body} onChange={(e) => setForm({ ...form, body: e.target.value })} required />
        )}
      </div>
      <p className="muted small">Инструкция будет видна вашей команде. AI-buddy начнёт отвечать по ней сразу после сохранения.</p>
      <div className="row-start">
        <button className="btn" disabled={m.isPending}>{m.isPending ? 'Сохраняю…' : 'Сохранить'}</button>
        <button type="button" className="link" onClick={() => onDone(doc?.id)}>Отмена</button>
      </div>
    </form>
  );
}
