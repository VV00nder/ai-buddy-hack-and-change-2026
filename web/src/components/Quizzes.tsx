import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api, type QuizInput } from '../api/client';
import type { QuizFull, QuizInfo, QuizQuestion, QuizResult } from '../api/schemas';
import { keys, usePerms } from '../hooks/queries';
import { useUi } from '../store/ui';
import { TeamQuizResults } from './QuizResults';
import { plural } from '../lib/format';

/** Раздел «Тесты»: проверка усвоенного материала после темы или блока тем. */
export function Quizzes({ role }: { role?: string }) {
  const { quizId, go } = useUi();
  const perms = usePerms();
  const [editing, setEditing] = useState<string | 'new' | null>(null);
  const list = useQuery({ queryKey: keys.quizzes, queryFn: api.quizzes });
  if (editing) return <QuizEditor id={editing === 'new' ? null : editing} onDone={() => setEditing(null)} />;
  if (quizId) return <QuizRunner id={quizId} onExit={() => go('quizzes')} />;
  if (list.isPending) return <p className="muted">Загружаю тесты…</p>;
  if (list.isError) return <p className="error">{list.error.message}</p>;
  const qs = list.data.quizzes;
  const passed = qs.filter((q) => q.passed).length;
  return (
    <section className="quizzes">
      <header className="row">
        <p className="muted">
          {role && role !== 'intern'
            ? 'Тесты после темы или блока тем. Ниже — результаты ваших стажёров; в карточке стажёра видны все попытки и ответ на каждый вопрос.'
            : `Короткие тесты после темы или блока тем. Сдано ${passed} из ${qs.length}. Ответы проверяются сразу, после сдачи — разбор ошибок.`}
        </p>
        {perms.has('kb') && <button className="btn small" onClick={() => setEditing('new')}>Новый тест</button>}
      </header>
      {role && role !== 'intern' && <TeamQuizResults />}
      {role && role !== 'intern' && <h3 className="section-title">Все тесты</h3>}
      <ul className="quiz-grid">
        {qs.map((q) => (
          <QuizCard key={q.id} q={q} onOpen={() => go('quizzes', { quizId: q.id })} onEdit={() => setEditing(q.id)} />
        ))}
      </ul>
    </section>
  );
}

function QuizCard({ q, onOpen, onEdit }: { q: QuizInfo; onOpen: () => void; onEdit: () => void }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const del = useMutation({
    mutationFn: () => api.deleteQuiz(q.id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.quizzes }),
    onError: (e) => toast(e.message, 'error'),
  });
  const state = q.passed ? 'passed' : q.attempts > 0 ? 'failed' : 'new';
  return (
    <li className={`quiz-card quiz-${state}`}>
      <div className="quiz-card-head">
        <h3>{q.title}</h3>
        {q.passed && <span className="stamp stamp-approved">сдан</span>}
      </div>
      {q.description && <p className="small muted">{q.description}</p>}
      <p className="small">
        {q.questions} {plural(q.questions, 'вопрос', 'вопроса', 'вопросов')}, проходной балл {q.passPercent}%
        {q.attempts > 0 && <span className="muted"> · лучший результат <span className="num">{q.best}%</span></span>}
      </p>
      <div className="row-start">
        <button className={q.passed ? 'btn ghost small' : 'btn small'} onClick={onOpen}>
          {q.passed ? 'Пройти ещё раз' : q.attempts > 0 ? 'Попробовать снова' : 'Начать'}
        </button>
        {q.editable && (
          <>
            <button className="link small" onClick={onEdit}>Изменить</button>
            <button className="link small danger" disabled={del.isPending} onClick={() => del.mutate()}>Удалить</button>
          </>
        )}
      </div>
    </li>
  );
}

export function QuizRunner({ id, onExit }: { id: string; onExit: () => void }) {
  const qc = useQueryClient();
  const go = useUi((s) => s.go);
  const toast = useUi((s) => s.toast);
  const q = useQuery({ queryKey: keys.quiz(id), queryFn: () => api.quiz(id) });
  const [answers, setAnswers] = useState<number[] | null>(null);
  const [result, setResult] = useState<QuizResult | null>(null);
  const submit = useMutation({
    mutationFn: (a: number[]) => api.submitQuiz(id, a),
    onSuccess: (r) => {
      setResult(r);
      // результат — вверху страницы: прокручиваем к нему
      if (typeof document !== 'undefined') document.querySelector('.content')?.scrollTo({ top: 0 });
      void qc.invalidateQueries({ queryKey: keys.quizzes });
      void qc.invalidateQueries({ queryKey: keys.plan });
      if (r.stepsDone.length > 0) toast('Шаг плана закрыт — тест сдан');
    },
    onError: (e) => toast(e.message, 'error'),
  });
  if (q.isPending) return <p className="muted">Загружаю тест…</p>;
  if (q.isError) return <p className="error">Тест недоступен: {q.error.message}</p>;
  const quiz = q.data;
  const a = answers ?? quiz.questions.map(() => -1);
  const answered = a.filter((x) => x >= 0).length;
  if (result) return <QuizReview result={result} docIds={quiz.docIds} onRetry={() => { setResult(null); setAnswers(null); }} onExit={onExit} />;
  function onSubmit(e: FormEvent) {
    e.preventDefault();
    submit.mutate(a);
  }
  return (
    <form className="quiz-run" onSubmit={onSubmit}>
      <button type="button" className="link back" onClick={onExit}>Все тесты</button>
      <h2>{quiz.title}</h2>
      {quiz.description && <p className="muted">{quiz.description}</p>}
      <div className="quiz-progress" aria-live="polite">
        <div className="progress" role="progressbar" aria-valuenow={answered} aria-valuemin={0} aria-valuemax={quiz.questions.length} aria-label="Отвечено">
          <span style={{ width: `${(answered * 100) / quiz.questions.length}%` }} />
        </div>
        <span className="small num">Отвечено {answered} из {quiz.questions.length}</span>
      </div>
      <ol className="questions">
        {quiz.questions.map((qq, i) => (
          <li key={i}>
            <fieldset>
              <legend>{qq.text}</legend>
              {qq.options.map((o, j) => (
                <label key={j} className={`option ${a[i] === j ? 'chosen' : ''}`}>
                  <input
                    type="radio"
                    name={`q${i}`}
                    checked={a[i] === j}
                    onChange={() => setAnswers(a.map((x, k) => (k === i ? j : x)))}
                  />
                  {o}
                </label>
              ))}
            </fieldset>
          </li>
        ))}
      </ol>
      <div className="row-start">
        <button className="btn" disabled={answered < quiz.questions.length || submit.isPending}>
          {submit.isPending ? 'Проверяю…' : 'Проверить ответы'}
        </button>
        {answered < quiz.questions.length && <span className="muted small">Ответь на все вопросы. Проходной балл — {quiz.passPercent}%.</span>}
      </div>
      {quiz.docIds.length > 0 && (
        <p className="small muted">
          Тема:{' '}
          {quiz.docIds.map((d, i) => (
            <span key={d}>
              {i > 0 && ', '}
              <button type="button" className="link" onClick={() => go('knowledge', { docId: d })}>инструкция {i + 1}</button>
            </span>
          ))}
        </p>
      )}
    </form>
  );
}

export function QuizReview({ result, docIds, onRetry, onExit }: { result: QuizResult; docIds: string[]; onRetry: () => void; onExit: () => void }) {
  const go = useUi((s) => s.go);
  const at = result.attempt;
  return (
    <section className="quiz-run">
      <button className="link back" onClick={onExit}>Все тесты</button>
      <div className={`quiz-score ${at.passed ? 'ok' : 'fail'}`}>
        <span className="quiz-score-num num">{at.percent}%</span>
        <div>
          <h2>{at.passed ? 'Тест сдан' : 'Пока не сдан'}</h2>
          <p>
            Правильно {at.correct} из {at.total}.{' '}
            {at.passed ? (result.stepsDone.length > 0 ? 'Шаг плана отмечен выполненным.' : 'Наставник увидит результат.') : 'Посмотри разбор и попробуй ещё раз.'}
          </p>
        </div>
      </div>
      <ol className="questions review">
        {result.review.map((r, i) => (
          <li key={i} className={r.right ? 'right' : 'wrong'}>
            <p className="q-text">{r.text}</p>
            <p className="small">
              {r.right ? 'Верно: ' : 'Твой ответ: '}
              {r.options[r.chosen] ?? 'нет ответа'}
            </p>
            {!r.right && <p className="small right-answer">Правильный ответ: {r.options[r.correct]}</p>}
            {r.explain && <p className="small muted">{r.explain}</p>}
          </li>
        ))}
      </ol>
      <div className="row-start">
        {!at.passed && <button className="btn" onClick={onRetry}>Пройти ещё раз</button>}
        {docIds[0] && <button className="btn ghost" onClick={() => go('knowledge', { docId: docIds[0] })}>Перечитать инструкцию</button>}
        {at.passed && <button className="btn ghost" onClick={() => go('plan')}>К плану</button>}
      </div>
    </section>
  );
}

const emptyQuestion = (): QuizQuestion => ({ text: '', options: ['', ''], correct: 0, explain: '' });

export function QuizEditor({ id, onDone }: { id: string | null; onDone: () => void }) {
  const full = useQuery({ queryKey: keys.quizFull(id ?? ''), queryFn: () => api.quizFull(id!), enabled: !!id });
  if (id && full.isPending) return <p className="muted">Загружаю тест…</p>;
  if (id && full.isError) return <p className="error">{full.error.message}</p>;
  return <QuizForm initial={full.data} onDone={onDone} />;
}

export function QuizForm({ initial, onDone }: { initial?: QuizFull; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useUi((s) => s.toast);
  const docs = useQuery({ queryKey: keys.docs(''), queryFn: () => api.docs('') });
  const [form, setForm] = useState<QuizInput>(() => ({
    title: initial?.title ?? '',
    description: initial?.description ?? '',
    docIds: initial?.docIds ?? [],
    passPercent: initial?.passPercent ?? 80,
    questions: initial?.questions.map((q) => ({ ...q, options: [...q.options] })) ?? [emptyQuestion()],
  }));
  const m = useMutation({
    mutationFn: () => api.saveQuiz(initial?.id ?? null, form),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.quizzes });
      toast('Тест сохранён');
      onDone();
    },
    onError: (e) => toast(e.message, 'error'),
  });
  const setQ = (i: number, patch: Partial<QuizQuestion>) =>
    setForm({ ...form, questions: form.questions.map((q, k) => (k === i ? { ...q, ...patch } : q)) });
  function submit(e: FormEvent) {
    e.preventDefault();
    m.mutate();
  }
  return (
    <form className="editor quiz-editor" onSubmit={submit}>
      <button type="button" className="link back" onClick={onDone}>Все тесты</button>
      <h2>{initial ? 'Редактирование теста' : 'Новый тест'}</h2>
      <div className="editor-grid">
        <label className="field">
          Название
          <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} required minLength={3} />
        </label>
        <label className="field">
          Проходной балл, %
          <input type="number" min={1} max={100} value={form.passPercent} onChange={(e) => setForm({ ...form, passPercent: Number(e.target.value) })} />
        </label>
      </div>
      <label className="field">
        Описание
        <input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
      </label>
      <fieldset className="doc-pick">
        <legend className="small muted">Какие инструкции проверяет тест</legend>
        <div className="doc-pick-list">
          {(docs.data?.docs ?? []).map((d) => (
            <label key={d.id} className="checkbox">
              <input
                type="checkbox"
                checked={form.docIds.includes(d.id)}
                onChange={() =>
                  setForm({ ...form, docIds: form.docIds.includes(d.id) ? form.docIds.filter((x) => x !== d.id) : [...form.docIds, d.id] })
                }
              />
              {d.title}
            </label>
          ))}
        </div>
      </fieldset>
      <ol className="q-edit-list">
        {form.questions.map((q, i) => (
          <li key={i} className="q-edit">
            <div className="row">
              <strong>Вопрос {i + 1}</strong>
              {form.questions.length > 1 && (
                <button type="button" className="link small danger" onClick={() => setForm({ ...form, questions: form.questions.filter((_, k) => k !== i) })}>
                  Удалить вопрос
                </button>
              )}
            </div>
            <label className="field">
              <span className="sr-only">Текст вопроса {i + 1}</span>
              <input value={q.text} onChange={(e) => setQ(i, { text: e.target.value })} placeholder="Текст вопроса" required />
            </label>
            <p className="small muted">Отметь правильный вариант</p>
            {q.options.map((o, j) => (
              <div key={j} className="opt-edit">
                <input type="radio" name={`correct-${i}`} checked={q.correct === j} onChange={() => setQ(i, { correct: j })} aria-label={`Вариант ${j + 1} правильный`} />
                <input value={o} onChange={(e) => setQ(i, { options: q.options.map((x, k) => (k === j ? e.target.value : x)) })} placeholder={`Вариант ${j + 1}`} required />
                {q.options.length > 2 && (
                  <button
                    type="button"
                    className="link small"
                    aria-label={`Удалить вариант ${j + 1}`}
                    onClick={() => setQ(i, { options: q.options.filter((_, k) => k !== j), correct: q.correct === j ? 0 : q.correct > j ? q.correct - 1 : q.correct })}
                  >
                    ✕
                  </button>
                )}
              </div>
            ))}
            {q.options.length < 6 && (
              <button type="button" className="link small" onClick={() => setQ(i, { options: [...q.options, ''] })}>+ вариант</button>
            )}
            <label className="field">
              Пояснение после ответа
              <input value={q.explain ?? ''} onChange={(e) => setQ(i, { explain: e.target.value })} placeholder="Почему этот ответ правильный" />
            </label>
          </li>
        ))}
      </ol>
      <button type="button" className="btn ghost small" onClick={() => setForm({ ...form, questions: [...form.questions, emptyQuestion()] })}>Добавить вопрос</button>
      <div className="row-start">
        <button className="btn" disabled={m.isPending}>{m.isPending ? 'Сохраняю…' : 'Сохранить тест'}</button>
        <button type="button" className="link" onClick={onDone}>Отмена</button>
      </div>
    </form>
  );
}
