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
check('прогресс цели посчитан по закрытому шагу плана', (await page.locator('.goal').first().locator('.pct').innerText()) === '20%');
await caption('Цель состоит из задач: процент считается сам. Задача-шаг «доступ к Stash» закрылась вместе с шагом плана', 3000);
await page.locator('.goal').first().locator('.goal-tasks input:not([disabled])').first().click();
await page.locator('.goal').first().locator('.pct', { hasText: '40%' }).waitFor();
check('задача цели закрыта — прогресс пересчитан', true);
await pause(900);
await page.locator('.step-done', { hasText: 'Stash' }).scrollIntoViewIfNeeded();
await caption('Шаг плана «Получи доступ к Stash» закрылся автоматически после выдачи доступа', 2600);
await page.getByRole('button', { name: 'Чат', exact: true }).click();
await caption('Ответ наставника теперь в базе знаний: следующий стажёр получит его сразу', 2000);
await ask('Где тестовый стенд поиска?');
check('повторный вопрос закрыт без наставника', (await lastAssistant().innerText()).includes('search-staging'));
await pause(1500);

// 8. Обучение: отметка шагов из чата и тест после темы
await caption('Шаги плана отмечаются прямо из чата: галочкой в панели «Сегодня» или словами', 2400);
await page.locator('.today .mini-check[aria-checked="false"]').first().click();
await page.locator('.today .mini-done').first().waitFor();
check('шаг отмечен в панели «Сегодня»', (await page.locator('.today .mini-done').count()) >= 1);
await ask('Я настроил avito-cli');
check('шаг отмечен из чата', (await lastAssistant().innerText()).includes('Отметил шаг'));
await caption('После темы — короткий тест. AI-buddy подбирает его по теме вопроса', 2200);
await ask('Проверь мои знания по SSH-ключам');
await lastAssistant().getByRole('button', { name: 'Пройти тест' }).click();
await page.locator('.quiz-run legend').first().waitFor();
check('во время теста чат и инструкции закрыты', await page.locator('.rail button', { hasText: 'Инструкции' }).isDisabled());
await caption('Пока идёт тест, чат и инструкции закрыты — подсмотреть материал нельзя', 2200);
const answersWeek1 = [1, 0, 1, 2, 0];
for (const [i, j] of answersWeek1.entries()) {
  await page.locator('.questions > li').nth(i).locator('.option').nth(j).click();
  await pause(350);
}
await caption('Ответы проверяет сервер — правильные варианты не уходят в браузер', 2000);
await page.getByRole('button', { name: 'Проверить ответы' }).click();
await page.locator('.quiz-score').waitFor();
check('тест сдан, шаг плана закрыт', (await page.locator('.quiz-score').innerText()).includes('Шаг плана отмечен'));
await caption('Тест сдан: шаг плана закрылся сам, наставник получил уведомление с результатом', 2800);
await logout();

// 9. Руководитель: права, инструкции, шаблоны обучения
await caption('Руководитель передаёт часть полномочий: инструкции и тесты, согласование заявок', 2400);
await loginAs('Мария Волкова');
await page.getByRole('button', { name: 'Отдел и права', exact: true }).click();
await page.getByRole('switch', { name: 'Согласование заявок: Елена Соколова' }).click();
await page.getByText('Право выдано').waitFor();
check('право на согласование выдано', await page.getByRole('switch', { name: 'Согласование заявок: Елена Соколова' }).isChecked());
await pause(1200);
await caption('Новая инструкция в папке команды — AI-buddy сразу отвечает по ней', 2200);
await page.getByRole('button', { name: 'Инструкции', exact: true }).click();
await page.getByRole('button', { name: 'Новая инструкция' }).click();
await page.getByLabel('Название').fill('Как проходит релиз поиска');
await page.getByLabel('Папка').fill('Процессы/Релизы');
await page.locator('#doc-body').fill('Релизы поиска — по вторникам после зелёного CI.\n\n## Чек-лист\n1. Зелёный CI.\n2. Ревью наставника.\n3. Выкатка на staging и проверка дашборда.');
await pause(1200);
await page.getByRole('button', { name: 'Сохранить', exact: true }).click();
await page.getByText('Инструкция команды').waitFor();
check('инструкция в папке «Процессы / Релизы»', (await page.locator('.crumbs').innerText()).includes('Релизы'));
await pause(1200);
await caption('Шаблоны обучения: встроенные можно скопировать, назначить стажёру и настроить под него', 2400);
await page.getByRole('button', { name: 'Стажёры', exact: true }).click();
await page.locator('.cab-interns .person').first().hover();
check('сводка о стажёре при наведении', await page.locator('.summary-pop').first().isVisible());
await caption('Кабинет: все стажёры, отметки о просрочках и сводка при наведении — без лишних переходов', 2600);
await page.locator('.cab-interns tbody tr').first().click();
await page.locator('.intern-requests').waitFor();
check('заявки стажёра в карточке', (await page.locator('.intern-requests .req').count()) >= 2);
await page.locator('.program-panel').waitFor();
await page.locator('#tpl-pick').selectOption({ label: 'Стажёр-аналитик' });
page.once('dialog', (d) => void d.accept());
await page.getByRole('button', { name: 'Назначить', exact: true }).click();
await page.getByText('Программа «Стажёр-аналитик»', { exact: true }).waitFor();
check('стажёру назначен шаблон «Стажёр-аналитик»', true);
await page.evaluate(() => document.querySelector('.content').scrollTo(0, 1e6));
check('страница не уходит ниже содержимого', await page.evaluate(() => document.scrollingElement.scrollHeight <= innerHeight + 1));
await pause(1500);
await logout();

await loginAs('Анна Смирнова');
await caption('Анна сразу получает ответ по новой инструкции команды', 1800);
await ask('Когда проходят релизы поиска?');
check('ответ по новой инструкции', (await lastAssistant().innerText()).includes('вторник'));
await pause(1500);
await logout();

// 10. Регистрация: только ФИО, почта и пароль; отдел и роль назначает руководитель
const email = `psidorov${Date.now()}@example.com`;
await caption('Новый сотрудник регистрируется сам: ФИО, почта и пароль', 2000);
await page.goto(BASE);
await page.getByRole('button', { name: 'Зарегистрироваться' }).click();
await page.getByLabel('ФИО').fill('Сидоров Пётр Андреевич');
await page.getByLabel('Рабочая почта').fill(email);
await page.getByLabel('Пароль').fill('secret2026');
await page.getByRole('button', { name: 'Зарегистрироваться' }).click();
await page.getByText('Ждём назначения').waitFor();
check('после регистрации — ожидание назначения', true);
await page.getByRole('button', { name: 'Выйти' }).click();
await page.getByText('Войти тестовой учётной записью').waitFor();
await loginAs('Мария Волкова');
await page.getByRole('button', { name: 'Отдел и права', exact: true }).click();
await page.getByText('Сидоров Пётр Андреевич').last().waitFor();
await caption('Руководитель назначает роль и наставника и принимает в отдел', 2200);
await page.getByRole('button', { name: 'Принять в отдел' }).last().click();
await page.getByText('принят(а) в отдел').waitFor();
check('руководитель принял сотрудника в отдел', true);
await logout();
await page.getByLabel('Логин или почта').fill(email);
await page.getByLabel('Пароль').fill('secret2026');
await page.getByRole('button', { name: 'Войти', exact: true }).click();
await page.getByRole('button', { name: 'Мой план', exact: true }).waitFor();
check('у нового стажёра открылся план', true);
await logout();

// 11. Польза
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
