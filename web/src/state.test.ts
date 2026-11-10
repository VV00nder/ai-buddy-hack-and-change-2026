import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { ChatState } from './api/schemas';
import { ApiError } from './api/client';
import { applyUpdate, byCreated, keys, makeQueryClient, upsertBy, upsertNewer } from './hooks/queries';
import { useUi } from './store/ui';
import { absence, msgs, plan } from './test-fixtures';

test('upsert helpers', () => {
  const a = { id: '1', status: 'pending', updatedAt: '2026-11-10T12:00:01Z' };
  const done = { id: '1', status: 'done', updatedAt: '2026-11-10T12:00:02Z' };
  assert.deepEqual(upsertBy([], a), [a]);
  assert.deepEqual(upsertNewer([done], a), [done]); // устаревшая версия не затирает свежую
  assert.deepEqual(upsertNewer([a], done), [done]);
  const sameTime = { ...a, updatedAt: done.updatedAt };
  assert.deepEqual(upsertNewer([done], sameTime), [done]);
  const sorted = byCreated([msgs[2]!, msgs[1]!, msgs[0]!]);
  assert.deepEqual(sorted.map((m) => m.id), ['m1', 'm2', 'm3']);
});

test('applyUpdate updates query cache', () => {
  const qc = makeQueryClient();
  qc.setQueryData<ChatState>(keys.chat, { messages: [msgs[0]!], actions: [] });
  applyUpdate(qc, { type: 'message', data: msgs[1] });
  applyUpdate(qc, { type: 'action', data: absence });
  applyUpdate(qc, { type: 'message', data: { broken: true } });
  const s = qc.getQueryData<ChatState>(keys.chat)!;
  assert.equal(s.messages.length, 2);
  assert.equal(s.actions[0]!.id, 'a1');
  applyUpdate(qc, { type: 'notification', data: { id: 'n', kind: 'action', text: 't', read: false, createdAt: '' } });
  assert.equal(qc.getQueryData<{ notifications: unknown[] }>(keys.notifications)!.notifications.length, 1);
  applyUpdate(qc, { type: 'plan', data: plan });
  assert.equal(qc.getQueryData<typeof plan>(keys.plan)!.day, 4);
  applyUpdate(qc, { type: 'approval', data: {} });
  applyUpdate(qc, { type: 'question', data: {} });
  applyUpdate(qc, { type: 'unknown', data: {} });
  const fresh = makeQueryClient();
  applyUpdate(fresh, { type: 'message', data: msgs[1] }); // кэша ещё нет — без ошибок
  const retry = fresh.getDefaultOptions().queries!.retry as (n: number, e: unknown) => boolean;
  assert.equal(retry(0, new ApiError(404, 'x', 'x')), false);
  assert.equal(retry(0, new ApiError(503, 'x', 'x')), true);
  assert.equal(retry(2, new Error('x')), false);
});

test('ui store', () => {
  const ui = useUi.getState();
  ui.go('knowledge', { docId: 'd1' });
  assert.equal(useUi.getState().view, 'knowledge');
  assert.equal(useUi.getState().docId, 'd1');
  ui.go('team');
  assert.equal(useUi.getState().docId, null);
  ui.toast('a');
  ui.toast('b', 'error');
  ui.toast('c');
  ui.toast('d');
  assert.equal(useUi.getState().toasts.length, 3);
  ui.dismiss(useUi.getState().toasts[0]!.id);
  assert.equal(useUi.getState().toasts.length, 2);
  ui.setOnline(false);
  ui.setDraft('x');
  assert.equal(useUi.getState().online, false);
  assert.equal(useUi.getState().draft, 'x');
});
