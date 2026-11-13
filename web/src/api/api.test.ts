import assert from 'node:assert/strict';
import { test } from 'node:test';
import { anna, msgs } from '../test-fixtures';
import { ApiError, createApi } from './client';
import { subscribe, type EventSourceLike } from './sse';

type Call = { url: string; init: RequestInit };

function fakeFetch(handler: (url: string, init: RequestInit) => { status?: number; body: unknown } | Error) {
  const calls: Call[] = [];
  const f = (async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    const r = handler(url, init);
    if (r instanceof Error) throw r;
    return new Response(typeof r.body === 'string' ? r.body : JSON.stringify(r.body), { status: r.status ?? 200 });
  }) as unknown as typeof fetch;
  return { f, calls };
}

test('client validates responses and sends CSRF header', async () => {
  const { f, calls } = fakeFetch(() => ({ body: { user: anna } }));
  const api = createApi(f);
  const me = await api.me();
  assert.equal(me.user.name, 'Анна Смирнова');
  assert.equal((calls[0]!.init.headers as Record<string, string>)['X-Requested-With'], 'ai-buddy');
  await api.login('anna', 'x');
  assert.equal(calls[1]!.init.method, 'POST');
  assert.deepEqual(JSON.parse(String(calls[1]!.init.body)), { login: 'anna', password: 'x' });
});

test('client maps errors', async () => {
  const http = createApi(fakeFetch(() => ({ status: 403, body: { error: 'forbidden', message: 'Недостаточно прав' } })).f);
  await assert.rejects(http.approvals(), (e: unknown) => e instanceof ApiError && e.status === 403 && e.message === 'Недостаточно прав');
  const bare = createApi(fakeFetch(() => ({ status: 500, body: 'oops' })).f);
  await assert.rejects(bare.plan(), (e: unknown) => e instanceof ApiError && e.code === 'error' && e.message === 'Ошибка 500');
  const schema = createApi(fakeFetch(() => ({ body: { nope: 1 } })).f);
  await assert.rejects(schema.plan(), (e: unknown) => e instanceof ApiError && e.code === 'schema');
  const net = createApi(fakeFetch(() => new Error('down')).f);
  await assert.rejects(net.me(), (e: unknown) => e instanceof ApiError && e.code === 'network');
});

test('client endpoints', async () => {
  const { f, calls } = fakeFetch((url) => {
    const ok = { ok: true };
    const map: Record<string, unknown> = {
      '/api/messages': { messages: msgs, actions: null },
      '/api/auth/demo-users': { users: [anna], password: 'p' },
    };
    return { body: map[url] ?? ok };
  });
  const api = createApi(f);
  const chat = await api.chat();
  assert.equal(chat.actions.length, 0);
  assert.equal(chat.messages[0]!.sources.length, 0);
  await api.demoUsers();
  await api.logout();
  await api.feedback('m 1', true);
  await api.readNotifications();
  await api.decide('absence', 'ABS-1', true, 'ok');
  for (const p of [api.config(), api.send('x'), api.actions(), api.confirm('a'), api.cancel('a'), api.setStep('s', 'done'), api.team(),
    api.internPlan('u'), api.approvals(), api.inbox(), api.answer('q', 'a', true), api.notifications(), api.insights(), api.docs(), api.doc('d'),
    api.dashboard(), api.internRequests('u 1'), api.reminders(), api.addReminder({ text: 'x', inHours: 1 }), api.deleteReminder('r1'),
    api.assign('u-new', 'intern', 'u-oleg'), api.startQuiz('q1'), api.cancelQuiz(), api.setGoalTask('g1', 't1', true),
    api.addGoalTask('g1', { title: 'MR' }), api.addGoal('u', { title: 'Цель', metric: 'м', due: '2026-12-01', tasks: [{ title: 'a' }] })]) {
    await p.catch(() => undefined); // ответы-заглушки не проходят схему — проверяем только адреса
  }
  const urls = calls.map((c) => `${c.init.method} ${c.url}`);
  assert.ok(urls.includes('POST /api/messages/m%201/feedback'));
  assert.ok(urls.includes('PATCH /api/plan/steps/s'));
  assert.ok(urls.includes('POST /api/approvals/absence/ABS-1'));
  assert.ok(urls.includes('POST /api/inbox/q/answer'));
  for (const u of ['GET /api/dashboard', 'GET /api/team/u%201/requests', 'POST /api/reminders', 'DELETE /api/reminders/r1', 'POST /api/department/u-new/assign',
    'POST /api/quizzes/q1/start', 'DELETE /api/quizzes/active', 'PATCH /api/goals/g1/tasks/t1', 'POST /api/goals/g1/tasks', 'POST /api/goals']) {
    assert.ok(urls.includes(u), u);
  }
});

test('sse subscription', () => {
  const es: EventSourceLike & { closed?: boolean } = { onmessage: null, onerror: null, onopen: null, close() { this.closed = true; } };
  const got: string[] = [];
  const status: boolean[] = [];
  const stop = subscribe((u) => got.push(u.type), (s) => status.push(s), () => es);
  es.onopen!({});
  es.onmessage!({ data: JSON.stringify({ type: 'message', data: {} }) });
  es.onmessage!({ data: 'not json' });
  es.onmessage!({ data: JSON.stringify({ nope: 1 }) });
  es.onerror!({});
  stop();
  assert.deepEqual(got, ['message']);
  assert.deepEqual(status, [true, false]);
  assert.equal(es.closed, true);
});
