import assert from 'node:assert/strict';
import { test } from 'node:test';
import { renderToStaticMarkup } from 'react-dom/server';
import { readFileSync } from 'node:fs';
import { registerServiceWorker, useInstallPrompt, viewFromSearch } from './pwa';

test('service worker registers only in production', async () => {
  const calls: string[] = [];
  const nav = { serviceWorker: { register: async (u: string) => { calls.push(u); } } };
  let onLoad: (() => void) | undefined;
  assert.equal(registerServiceWorker(nav, false, (cb) => (onLoad = cb)), false);
  assert.equal(registerServiceWorker({}, true, (cb) => (onLoad = cb)), false);
  assert.equal(registerServiceWorker(nav, true, (cb) => (onLoad = cb)), true);
  onLoad!();
  assert.deepEqual(calls, ['/sw.js']);
  const failing = { serviceWorker: { register: () => Promise.reject(new Error('x')) } };
  registerServiceWorker(failing, true, (cb) => cb()); // ошибка регистрации не всплывает
  assert.equal(registerServiceWorker(undefined, false), false);
});

test('shortcut view from query string', () => {
  assert.equal(viewFromSearch('?view=plan'), 'plan');
  assert.equal(viewFromSearch('?view=hack'), null);
  assert.equal(viewFromSearch(''), null);
});

test('install prompt hook renders without target', () => {
  function Probe() {
    const { canInstall } = useInstallPrompt(undefined);
    return <span>{String(canInstall)}</span>;
  }
  assert.equal(renderToStaticMarkup(<Probe />), '<span>false</span>');
});

test('manifest is valid', () => {
  const m = JSON.parse(readFileSync('public/manifest.webmanifest', 'utf8'));
  assert.equal(m.display, 'standalone');
  assert.ok(m.icons.some((i: { purpose: string }) => i.purpose === 'maskable'));
  assert.ok(m.shortcuts.every((s: { url: string }) => viewFromSearch(s.url.slice(1)) !== null));
});
