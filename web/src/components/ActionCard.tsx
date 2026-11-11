import { useState } from 'react';
import type { Action } from '../api/schemas';
import { useCancel, useConfirm } from '../hooks/queries';
import { dayMonth, kindName, levelName, stampText } from '../lib/format';

const str = (v: unknown) => (typeof v === 'string' ? v : v == null ? '' : String(v));

/** Поля, которые можно поправить перед отправкой. */
export function editableFields(a: Action): { key: string; label: string; type: 'date' | 'text' | 'textarea' }[] {
  if (a.kind === 'absence')
    return [
      { key: 'date_from', label: 'С', type: 'date' },
      { key: 'date_to', label: 'По', type: 'date' },
      { key: 'reason', label: 'Причина (по желанию)', type: 'text' },
    ];
  if (a.kind === 'access') return a.preview.approval === 'auto' ? [] : [{ key: 'justification', label: 'Для какой задачи (увидит согласующий)', type: 'text' }];
  return [{ key: 'question', label: 'Вопрос', type: 'textarea' }];
}

/** Строки «что будет дальше» из предпросмотра сервиса. */
export function facts(a: Action): [string, string][] {
  const p = { ...a.preview, ...a.result };
  const out: [string, string][] = [];
  if (a.kind === 'absence') {
    if (p.days != null) out.push(['Рабочих дней', str(p.days)]);
    if (p.approver) out.push(['Согласует', str(p.approver)]);
    if (p.balance_after != null && a.status === 'proposed') out.push(['Останется отгулов', str(p.balance_after)]);
    if (p.comment) out.push(['Комментарий', str(p.comment)]);
  } else if (a.kind === 'access') {
    if (p.project) out.push(['Проект', str(p.project)]);
    const lvl = str(p.level ?? a.args.level ?? '');
    out.push(['Уровень', levelName[lvl] ?? lvl]);
    if (p.approver) out.push(['Согласует', p.approver_role ? `${str(p.approver)}, ${str(p.approver_role)}` : str(p.approver)]);
    if (p.approval === 'auto' && a.status === 'proposed') out.push(['Как выдаётся', 'Сразу после отправки']);
    if (p.comment) out.push(['Комментарий', str(p.comment)]);
  } else {
    if (p.mentor) out.push(['Кому', str(p.mentor)]);
    if (p.answer) out.push(['Ответ', str(p.answer)]);
  }
  if (a.externalId && a.kind !== 'mentor_question') out.push(['Номер заявки', a.externalId]);
  return out;
}

export function ActionCard({ action: a }: { action: Action }) {
  const confirm = useConfirm();
  const cancel = useCancel();
  const fields = editableFields(a);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const value = (k: string) => edits[k] ?? str(a.args[k]);
  const busy = confirm.isPending || cancel.isPending;
  const proposed = a.status === 'proposed';
  const changed = Object.keys(edits).filter((k) => edits[k] !== str(a.args[k]));

  return (
    <article className={`note note-${a.kind} status-${a.status}`} aria-label={`${kindName[a.kind]}: ${a.title}`}>
      <header>
        <span className="note-kind">{kindName[a.kind]}</span>
        {!proposed && <span className={`stamp stamp-${a.status}`}>{stampText(a)}</span>}
      </header>
      <h3>{a.title}</h3>
      {proposed ? (
        <div className="note-fields">
          {fields.map((f) => (
            <label key={f.key} className={`field field-${f.type}`}>
              <span>{f.label}</span>
              {f.type === 'textarea' ? (
                <textarea value={value(f.key)} onChange={(e) => setEdits({ ...edits, [f.key]: e.target.value })} rows={3} maxLength={1000} />
              ) : (
                <input type={f.type} value={value(f.key)} onChange={(e) => setEdits({ ...edits, [f.key]: e.target.value })} maxLength={300} />
              )}
            </label>
          ))}
        </div>
      ) : (
        a.kind === 'absence' && (
          <p className="note-dates">
            {dayMonth(str(a.args.date_from))}
            {str(a.args.date_to) && a.args.date_to !== a.args.date_from ? ` — ${dayMonth(str(a.args.date_to))}` : ''}
          </p>
        )
      )}
      {facts(a).length > 0 && (
        <dl className="facts">
          {facts(a).map(([k, v]) => (
            <div key={k}>
              <dt>{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>
      )}
      {a.status === 'failed' && <p className="error">{a.error}</p>}
      {proposed && (
        <div className="note-actions">
          <button className="btn" disabled={busy} onClick={() => confirm.mutate({ id: a.id, edits: Object.fromEntries(changed.map((k) => [k, edits[k]!])) })}>
            {confirm.isPending ? 'Отправляю…' : a.kind === 'mentor_question' ? 'Отправить наставнику' : 'Отправить заявку'}
          </button>
          <button className="btn ghost" disabled={busy} onClick={() => cancel.mutate(a.id)}>
            Не нужно
          </button>
          {changed.length > 0 && <span className="muted small">Сервис ещё раз проверит изменённые поля при отправке</span>}
        </div>
      )}
    </article>
  );
}
