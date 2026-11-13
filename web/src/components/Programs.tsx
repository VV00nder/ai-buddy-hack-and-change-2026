import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { Period, Template, TemplateStep } from '../api/schemas';
import { keys, usePerms } from '../hooks/queries';
import { plural } from '../lib/format';
import { useUi } from '../store/ui';

/** Раздел «Шаблоны обучения»: встроенные шаблоны и шаблоны команды. */
export function Programs() {
  const list = useQuery({ queryKey: keys.templates, queryFn: api.templates });
  const [edit, setEdit] = useState<Template | null>(null);
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const del = useMutation({
    mutationFn: (id: string) => api.deleteTemplate(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.templates });
      toast('Шаблон удалён. Программы стажёров, созданные по нему, не изменились.');
    },
    onError: (e) => toast(e.message, 'error'),
  });
  if (edit) return <TemplateEditor initial={edit} onDone={() => setEdit(null)} />;
  if (list.isPending) return <p className="muted">Загружаю шаблоны…</p>;
  if (list.isError) return <p className="error">{list.error.message}</p>;
  const copy = (t: Template): Template => ({ ...t, id: '', builtin: false, name: `${t.name} (копия)`, steps: t.steps.map((s) => ({ ...s })) });
  return (
    <section className="programs">
      <p className="muted">
        Шаблон — набор этапов и шагов адаптации. Назначьте шаблон стажёру во вкладке «Стажёры» и при необходимости настройте программу под него.
        Встроенные шаблоны можно скопировать и изменить.
      </p>
      <ul className="tpl-grid">
        {list.data.templates.map((t) => (
          <li key={t.id} className="tpl-card">
            <div className="row">
              <h3>{t.name}</h3>
              <span className="tag">{t.builtin ? 'встроенный' : 'команды'}</span>
            </div>
            {t.description && <p className="small muted">{t.description}</p>}
            <p className="small">
              {t.steps.length} {plural(t.steps.length, 'шаг', 'шага', 'шагов')}, тестов: {t.steps.filter((s) => s.quiz).length}
              {t.updatedBy && <span className="muted"> · изменил(а) {t.updatedBy}</span>}
            </p>
            <div className="row-start">
              {!t.builtin && <button className="btn small" onClick={() => setEdit(t)}>Изменить</button>}
              <button className={t.builtin ? 'btn small' : 'btn ghost small'} onClick={() => setEdit(copy(t))}>Сделать копию</button>
              {!t.builtin && (
                <button className="link small danger" disabled={del.isPending} onClick={() => del.mutate(t.id)}>Удалить</button>
              )}
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function TemplateEditor({ initial, onDone }: { initial: Template; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description);
  const save = useMutation({
    mutationFn: (steps: TemplateStep[]) => api.saveTemplate({ id: initial.id || undefined, name, description, periods: initial.periods, steps }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.templates });
      toast('Шаблон сохранён');
      onDone();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  return (
    <section className="editor">
      <button className="link back" onClick={onDone}>Все шаблоны</button>
      <h2>{initial.id ? 'Редактирование шаблона' : 'Новый шаблон'}</h2>
      <div className="editor-grid">
        <label className="field">
          Название
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="field">
          Описание
          <input value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
      </div>
      <StepEditor periods={initial.periods} steps={initial.steps} saving={save.isPending} saveLabel="Сохранить шаблон" onSave={(s) => save.mutate(s)} onCancel={onDone} />
    </section>
  );
}

/** Редактор шагов по этапам — для шаблонов и программ стажёров. */
export function StepEditor({
  periods,
  steps: initial,
  saving,
  saveLabel,
  onSave,
  onCancel,
  initialOpen = null,
}: {
  initialOpen?: number | null;
  periods: Period[];
  steps: TemplateStep[];
  saving: boolean;
  saveLabel: string;
  onSave: (steps: TemplateStep[]) => void;
  onCancel: () => void;
}) {
  const [steps, setSteps] = useState<TemplateStep[]>(() => initial.map((s) => ({ ...s })));
  const [open, setOpen] = useState<number | null>(initialOpen);
  const quizzes = useQuery({ queryKey: keys.quizzes, queryFn: api.quizzes });
  const set = (i: number, patch: Partial<TemplateStep>) => setSteps(steps.map((s, k) => (k === i ? { ...s, ...patch } : s)));
  const add = (period: string) => {
    const s: TemplateStep = { id: '', period, owner: 'intern', category: 'Обучение', title: '', description: '' };
    // новый шаг — в конец своего этапа
    const last = steps.map((x) => x.period).lastIndexOf(period);
    const next = [...steps];
    next.splice(last + 1, 0, s);
    setSteps(next);
    setOpen(last + 1);
  };
  const move = (i: number, d: -1 | 1) => {
    const j = i + d;
    if (j < 0 || j >= steps.length || steps[j]!.period !== steps[i]!.period) return;
    const next = [...steps];
    [next[i], next[j]] = [next[j]!, next[i]!];
    setSteps(next);
    setOpen(open === i ? j : open);
  };
  function submit(e: FormEvent) {
    e.preventDefault();
    onSave(steps);
  }
  return (
    <form className="step-editor" onSubmit={submit}>
      {periods.map((p) => (
        <fieldset key={p.id} className="se-period">
          <legend>
            {p.name} <span className="muted small">дни {p.fromDay + 1}–{p.toDay + 1}</span>
          </legend>
          <ul>
            {steps.map((s, i) =>
              s.period !== p.id ? null : (
                <li key={s.id || `new-${i}`} className={`se-step ${open === i ? 'open' : ''}`}>
                  <div className="se-row">
                    <span className="se-title">
                      {s.title || <span className="muted">Новый шаг</span>}
                      {s.owner === 'mentor' && <span className="tag">наставник</span>}
                      {s.quiz && <span className="tag quiz-tag">тест</span>}
                    </span>
                    <span className="se-actions">
                      <button type="button" className="link small" aria-label={`Выше: ${s.title}`} onClick={() => move(i, -1)}>↑</button>
                      <button type="button" className="link small" aria-label={`Ниже: ${s.title}`} onClick={() => move(i, 1)}>↓</button>
                      <button type="button" className="link small" onClick={() => setOpen(open === i ? null : i)}>{open === i ? 'Свернуть' : 'Изменить'}</button>
                      <button type="button" className="link small danger" onClick={() => setSteps(steps.filter((_, k) => k !== i))}>Убрать</button>
                    </span>
                  </div>
                  {open === i && (
                    <div className="se-fields">
                      <label className="field">
                        Название шага
                        <input value={s.title} onChange={(e) => set(i, { title: e.target.value })} required minLength={3} autoFocus={!s.id} />
                      </label>
                      <label className="field">
                        Описание
                        <textarea rows={2} value={s.description} onChange={(e) => set(i, { description: e.target.value })} />
                      </label>
                      <div className="editor-grid three">
                        <label className="field">
                          Кто отмечает
                          <select value={s.owner} onChange={(e) => set(i, { owner: e.target.value as TemplateStep['owner'] })}>
                            <option value="intern">Стажёр</option>
                            <option value="mentor">Наставник</option>
                          </select>
                        </label>
                        <label className="field">
                          Категория
                          <input value={s.category} onChange={(e) => set(i, { category: e.target.value })} />
                        </label>
                        <label className="field">
                          Закрывается тестом
                          <select value={s.quiz ?? ''} onChange={(e) => set(i, { quiz: e.target.value || undefined })}>
                            <option value="">— без теста —</option>
                            {(quizzes.data?.quizzes ?? []).map((q) => (
                              <option key={q.id} value={q.id}>{q.title}</option>
                            ))}
                          </select>
                        </label>
                      </div>
                      <label className="field">
                        Подсказка для чата (необязательно)
                        <input value={s.prompt ?? ''} onChange={(e) => set(i, { prompt: e.target.value || undefined })} placeholder="Например: Запроси доступ к Stash" />
                      </label>
                    </div>
                  )}
                </li>
              ),
            )}
          </ul>
          <button type="button" className="link small" onClick={() => add(p.id)}>+ шаг в «{p.name}»</button>
        </fieldset>
      ))}
      <div className="row-start sticky-actions">
        <button className="btn" disabled={saving}>{saving ? 'Сохраняю…' : saveLabel}</button>
        <button type="button" className="link" onClick={onCancel}>Отмена</button>
        <span className="muted small">{steps.length} {plural(steps.length, 'шаг', 'шага', 'шагов')}</span>
      </div>
    </form>
  );
}

/** Программа стажёра: назначенный шаблон и настройка под человека. */
export function ProgramPanel({ internId, internName }: { internId: string; internName: string }) {
  const perms = usePerms();
  const canEdit = perms.has('programs');
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const prog = useQuery({ queryKey: keys.program(internId), queryFn: () => api.program(internId) });
  const tpls = useQuery({ queryKey: keys.templates, queryFn: api.templates, enabled: canEdit });
  const [pick, setPick] = useState('');
  const [editing, setEditing] = useState(false);
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: keys.program(internId) });
    void qc.invalidateQueries({ queryKey: keys.internPlan(internId) });
    void qc.invalidateQueries({ queryKey: keys.team });
  };
  const assign = useMutation({
    mutationFn: (id: string) => api.assignTemplate(internId, id),
    onSuccess: (p) => {
      refresh();
      setPick('');
      toast(`Программа «${p.template.name}» назначена`);
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const save = useMutation({
    mutationFn: (steps: TemplateStep[]) => api.saveProgram(internId, steps),
    onSuccess: () => {
      refresh();
      setEditing(false);
      toast('Программа обновлена — стажёр получил уведомление');
    },
    onError: (e) => toast(e.message, 'error'),
  });
  if (prog.isPending) return null;
  if (prog.isError) return <p className="error">{prog.error.message}</p>;
  const p = prog.data;
  return (
    <section className="program-panel" aria-label="Программа обучения">
      <div className="row">
        <div>
          <h3>Программа обучения</h3>
          <p className="small">
            {p.template.name}
            {p.customized && <span className="tag custom-tag">настроена под стажёра</span>}
            {p.updatedBy && <span className="muted"> · {p.updatedBy}</span>}
          </p>
        </div>
        {canEdit && !editing && (
          <div className="row-start">
            <label className="sr-only" htmlFor="tpl-pick">Шаблон</label>
            <select id="tpl-pick" value={pick} onChange={(e) => setPick(e.target.value)}>
              <option value="">Назначить другой шаблон…</option>
              {(tpls.data?.templates ?? []).map((t) => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </select>
            <button
              className="btn ghost small"
              disabled={!pick || assign.isPending}
              onClick={() => {
                if (typeof window === 'undefined' || window.confirm(`Заменить программу ${internName}? Отметки совпадающих шагов сохранятся.`)) assign.mutate(pick);
              }}
            >
              Назначить
            </button>
            <button className="btn small" onClick={() => setEditing(true)}>Настроить под стажёра</button>
          </div>
        )}
      </div>
      {editing && (
        <StepEditor
          periods={p.template.periods}
          steps={p.template.steps}
          saving={save.isPending}
          saveLabel="Сохранить программу"
          onSave={(s) => save.mutate(s)}
          onCancel={() => setEditing(false)}
        />
      )}
    </section>
  );
}
