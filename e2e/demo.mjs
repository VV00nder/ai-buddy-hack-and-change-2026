// Сквозной сценарий AI-buddy (e2e) с записью видеодемонстрации.
// Запуск: стенд на http://localhost:8080, затем
//   node e2e/demo.mjs [--base http://localhost:8080] [--video out-dir] [--slow 1]
// Скрипт проверяет ключевые результаты и падает с ненулевым кодом при ошибке.
import { chromium } from 'playwright';

const arg = (name, def) => {
  const i = process.argv.indexOf(name);
  return i > -1 ? process.argv[i + 1] : def;
};
const BASE = arg('--base', 'http://localhost:8080');
const CRON = arg('--cron-token', 'demo-cron');
const VIDEO = arg('--video', '');
const SLOW = Number(arg('--slow', VIDEO ? '1.4' : '0'));
const pause = (ms) => new Promise((r) => setTimeout(r, ms * SLOW));

const browser = await chromium.launch();
const context = await browser.newContext({
  viewport: { width: 1280, height: 800 },
  locale: 'ru-RU',
  ...(VIDEO ? { recordVideo: { dir: VIDEO, size: { width: 1280, height: 800 } } } : {}),
});
// Субтитры поверх интерфейса — объясняют зрителю, что происходит.
await context.addInitScript(() => {
  window.__caption = (text) => {
    let el = document.getElementById('__caption');
    if (!el) {
      el = document.createElement('div');
      el.id = '__caption';
      el.style.cssText = 'position:fixed;left:50%;top:14px;transform:translateX(-50%);z-index:99999;max-width:900px;padding:10px 18px;border-radius:10px;background:rgba(20,20,28,.88);color:#fff;font:600 17px/1.35 system-ui,sans-serif;text-align:center;pointer-events:none;box-shadow:0 10px 30px -10px rgba(0,0,0,.5)';
      document.body.appendChild(el);
    }
    el.textContent = text;
    el.style.display = text ? 'block' : 'none';
  };
});
const page = await context.newPage();
const checks = [];
function check(name, ok) {
  checks.push([name, ok]);
  console.log(`${ok ? '✓' : '✗'} ${name}`);
}
async function caption(text, ms = 2200) {
  await page.evaluate((t) => window.__caption?.(t), text).catch(() => undefined);
  await pause(ms);
}
async function loginAs(name) {
  await page.goto(BASE);
  await page.getByRole('button', { name: new RegExp(name) }).click();
  await page.getByRole('navigation', { name: 'Разделы' }).waitFor();
  await pause(500);
}
async function logout() {
  await page.getByRole('button', { name: 'Выйти' }).click();
  await page.getByText('Войти тестовой учётной записью').waitFor();
}
async function ask(text) {
  const box = page.getByLabel('Сообщение');
  await box.fill('');
  await box.pressSequentially(text, { delay: VIDEO ? 25 : 0 });
  const before = await page.locator('.bubble.assistant:not(.pending)').count();
  await page.getByRole('button', { name: 'Отправить', exact: true }).click();
  await page.waitForFunction((n) => document.querySelectorAll('.bubble.assistant:not(.pending)').length > n, before, { timeout: 30000 });
  await pause(1200);
}
const lastAssistant = () => page.locator('.bubble.assistant').last();

// 1. Стажёр: вход и вопрос о процедуре
await page.goto(BASE);
await caption('AI-buddy — помощник стажёра. Входим как стажёр Анна, 4-й день стажировки', 2600);
await loginAs('Анна Смирнова');
await caption('Справа — план адаптации по чек-листу Авито: день, этап и следующий шаг');
await caption('Сценарий из кейса: стажёру нужен отгул, но он не знает порядок и стоит ли отвлекать руководителя', 2400);
await page.getByRole('button', { name: /Мне нужен отгул/ }).click();
await page.locator('.bubble.assistant:not(.pending) .source').first().waitFor({ timeout: 30000 });
await pause(800);
check('ответ о порядке оформления со ссылкой на инструкцию', (await lastAssistant().locator('.source').count()) > 0);
await caption('Ответ из инструкции команды с источником. Руководителя отвлекать не нужно — заявка уйдёт ему сама', 3000);

// 2. Подготовка и отправка заявки
await caption('Нажимаем подсказку — AI-buddy проверяет правила в системе отсутствий через MCP', 2200);
await page.locator('.chips.inline .chip').first().click();
await page.locator('.note.status-proposed').last().waitFor();
await pause(900);
check('черновик заявки на отгул', (await page.locator('.note-absence.status-proposed').count()) === 1);
await caption('Черновик заявки: даты, кто согласует, остаток отгулов. Ничего не отправлено без подтверждения', 2800);
await page.getByLabel('Причина (по желанию)').fill('Сессия в университете');
await pause(600);
await page.getByRole('button', { name: 'Отправить заявку' }).click();
await page.locator('.note-absence .stamp').waitFor();
check('заявка на отгул отправлена', (await page.locator('.note-absence .stamp').innerText()).includes('согласовании'));
await caption('Заявка создана в корпоративном сервисе и ждёт решения руководителя', 2400);

// 3. Доступы и права
await caption('Второй сценарий — доступы. В заявке проект, уровень прав и обоснование; согласующий зависит от ресурса', 2400);
await ask('Запроси доступ к stash на запись');
check('ассистент уточняет проект', (await page.locator('.chips.inline .chip').count()) === 3);
await caption('Проект не указан — AI-buddy предлагает проекты команды', 1800);
await page.locator('.chips.inline .chip', { hasText: 'search-api' }).click();
await page.locator('.note-access.status-proposed').waitFor();
await pause(800);
check('согласует владелец проекта', (await page.locator('.note-access.status-proposed').innerText()).includes('владелец проекта'));
await caption('Репозиторий согласует владелец проекта — наставник Олег, а не руководитель', 2400);
await page.getByRole('button', { name: 'Отправить заявку' }).click();
await page.locator('.note-access .stamp').last().waitFor();
check('заявка на доступ к Stash', true);
await caption('Права проверяет сам сервис: продовый Kubernetes стажёру не положен', 1800);
await ask('А дай доступ к kubernetes');
check('отказ по правам без черновика', (await lastAssistant().innerText()).includes('staging'));
await pause(1500);

// 4. Вопрос без ответа — наставнику
await caption('Если в инструкциях нет ответа, AI-buddy не выдумывает, а предлагает спросить наставника', 2200);
await ask('Где посмотреть тестовый стенд поиска?');
await page.getByRole('button', { name: 'Отправить наставнику' }).click();
await page.locator('.note-mentor_question .stamp').waitFor();
check('вопрос передан наставнику', true);
await pause(1200);
await logout();

// 5. Руководитель согласует
await caption('Руководитель Мария видит заявки и решает в один клик', 1800);
await loginAs('Мария Волкова');
await page.locator('.approval').first().waitFor();
check('у руководителя только отгул', (await page.locator('.approval').count()) === 1);
await pause(1200);
await page.locator('.approval').first().getByPlaceholder('Необязательно').fill('Хорошего отдыха!');
await page.locator('.approval').first().getByRole('button', { name: 'Согласовать' }).click();
await page.getByText('Нечего согласовывать').waitFor();
await pause(1000);
await logout();

// 6. Наставник отвечает и пополняет базу знаний
await caption('Олег — наставник и владелец репозитория: согласует доступ и отвечает на вопрос', 2200);
await loginAs('Олег Кузнецов');
await page.getByRole('button', { name: 'Согласования', exact: true }).click();
await page.locator('.approval').first().waitFor();
await pause(800);
await page.locator('.approval').first().getByRole('button', { name: 'Согласовать' }).click();
await page.getByText('Нечего согласовывать').waitFor();
await pause(600);
await page.getByRole('button', { name: 'Вопросы стажёров', exact: true }).click();
await page.getByPlaceholder('Ответ стажёру').fill('Стенд поиска — search-staging, ссылка в README сервиса. Доступ есть у всех инженеров команды.');
await pause(800);
await page.getByRole('button', { name: 'Ответить' }).click();
await page.getByText('Новых вопросов нет').waitFor();
await pause(1000);
await logout();

// 7. ИИ-крон и результат для стажёра
const cron = await fetch(BASE + '/api/cron/reminders', { method: 'POST', headers: { 'X-Cron-Token': CRON } }).then((r) => r.json());
check('ИИ-крон отправил утренние сводки', cron.digests >= 1);
await caption('Наутро внутренний ИИ-крон запускает напоминания. Анна открывает AI-buddy', 2200);
await loginAs('Анна Смирнова');
check('утренняя сводка в чате', (await page.locator('.bubble.proactive').count()) === 1);
await page.locator('.bubble.proactive').scrollIntoViewIfNeeded();
await caption('Утренняя сводка: день стажировки и следующий шаг. Плюс статусы заявок и ответ наставника', 2800);
const badge = await page.locator('.badge').innerText().catch(() => '0');
check('уведомления о решениях', Number(badge) >= 3);
await page.getByRole('button', { name: /Уведомления/ }).click();
await caption('Уведомления: отгул согласован с комментарием, доступ выдан, наставник ответил', 3000);
await page.getByRole('button', { name: /Уведомления/ }).click();
check('ответ наставника в чате', (await page.locator('.bubble.from-mentor').count()) === 1);
await page.getByRole('button', { name: 'Мой план', exact: true }).click();
await pause(800);
check('шаг плана про Stash закрыт автоматически', (await page.locator('.step-done', { hasText: 'Stash' }).count()) === 1);
check('цели стажировки в плане', (await page.locator('.goal').count()) === 3);
await caption('Цели стажировки с критериями успеха — трекер прогресса из чек-листа. Руководитель видит прогресс без отчётов', 3000);
await page.locator('.goal').first().getByRole('button', { name: 'Отметить выполненной' }).click();
await pause(900);
await page.locator('.step-done', { hasText: 'Stash' }).scrollIntoViewIfNeeded();
await caption('Шаг плана «Получи доступ к Stash» закрылся автоматически после выдачи доступа', 2600);
await page.getByRole('button', { name: 'Чат', exact: true }).click();
await caption('Ответ наставника теперь в базе знаний: следующий стажёр получит его сразу', 2000);
await ask('Где тестовый стенд поиска?');
check('повторный вопрос закрыт без наставника', (await lastAssistant().innerText()).includes('search-staging'));
await pause(1500);
await logout();

// 8. Польза
await caption('Метрики пользы: доля вопросов без наставника и сэкономленные часы', 1800);
await loginAs('Мария Волкова');
await page.getByRole('button', { name: 'Польза', exact: true }).click();
await page.locator('.saved-num').waitFor();
check('метрики пользы', (await page.locator('.kpis dd').count()) >= 6);
await caption('AI-buddy снимает с наставника рутину, но не заменяет живое общение', 3500);

const video = page.video();
await context.close();
await browser.close();
if (video) console.log('video:', await video.path());
const failed = checks.filter(([, ok]) => !ok);
console.log(`\n${checks.length - failed.length}/${checks.length} проверок пройдено`);
process.exit(failed.length ? 1 : 0);
