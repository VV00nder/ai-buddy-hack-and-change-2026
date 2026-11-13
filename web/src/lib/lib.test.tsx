import assert from 'node:assert/strict';
import { test } from 'node:test';
import { renderToStaticMarkup } from 'react-dom/server';
import { dayMonth, initials, plural, relative, remindAt, stampText, time, waited, firstName } from './format';
import { inline, Markdown, parseBlocks } from './markdown';

test('format helpers', () => {
  assert.equal(dayMonth('2026-11-13'), '13 ноября');
  assert.equal(firstName('Сидоров Пётр Андреевич'), 'Пётр');
  assert.equal(firstName('Анна Смирнова'), 'Анна');
  assert.equal(firstName(''), 'коллега');
  assert.equal(waited(0), 'меньше часа');
  assert.equal(waited(5), '5 ч');
  assert.equal(waited(50), '2 дн');
  assert.deepEqual(remindAt('3'), { inHours: 3 });
  const t = remindAt('tomorrow', new Date(2026, 10, 13, 18, 0));
  assert.equal(new Date(t.at!).getDate(), 14);
  assert.equal(new Date(t.at!).getHours(), 9);
  assert.equal(dayMonth('bad'), 'bad');
  assert.equal(initials('Анна Смирнова'), 'АС');
  assert.equal(plural(1, 'день', 'дня', 'дней'), 'день');
  assert.equal(plural(3, 'день', 'дня', 'дней'), 'дня');
  assert.equal(plural(12, 'день', 'дня', 'дней'), 'дней');
  assert.equal(time('bad'), '');
  assert.match(time('2026-11-10T12:05:00+03:00'), /\d{2}:\d{2}/);
  const now = new Date('2026-11-10T12:00:00Z');
  assert.equal(relative('2026-11-10T11:59:30Z', now), 'только что');
  assert.equal(relative('2026-11-10T11:30:00Z', now), '30 мин назад');
  assert.equal(relative('2026-11-10T09:00:00Z', now), '3 ч назад');
  assert.equal(relative('2026-11-01T09:00:00Z', now), '1 ноября');
  assert.equal(relative('bad', now), '');
  assert.equal(stampText({ kind: 'access', status: 'approved' }), 'Выдан');
  assert.equal(stampText({ kind: 'mentor_question', status: 'approved' }), 'Есть ответ');
  assert.equal(stampText({ kind: 'absence', status: 'approved' }), 'Согласовано');
  assert.equal(stampText({ kind: 'mentor_question', status: 'submitted' }), 'У наставника');
  assert.equal(stampText({ kind: 'absence', status: 'rejected' }), 'Отклонено');
});

test('markdown parser', () => {
  const b = parseBlocks('Абзац\nвторая строка\n\n- один\n- два\n1. раз\n2) два\n\nконец');
  assert.deepEqual(b.map((x) => x.type), ['p', 'ul', 'ol', 'p']);
  const html = renderToStaticMarkup(<Markdown text={'**жирный** и `код` и _курсив_\n<script>alert(1)</script>\n- a\n1. b'} />);
  assert.match(html, /<strong>жирный<\/strong>/);
  assert.match(html, /<code>код<\/code>/);
  assert.match(html, /<em>курсив<\/em>/);
  assert.match(html, /<br\/>/);
  assert.doesNotMatch(html, /<script>/); // HTML экранируется
  assert.match(html, /<ul><li>a<\/li><\/ul><ol><li>b<\/li><\/ol>/);
  assert.equal(inline('простой текст').length, 1);
});
