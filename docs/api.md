# HTTP API

Все ответы — JSON. Изменяющие запросы требуют заголовок `X-Requested-With: ai-buddy`. Авторизация — cookie `buddy_session` после входа.

| Метод | Путь | Роль | Описание |
|---|---|---|---|
| GET | `/api/health`, `/api/ready` | — | Живость и готовность (PostgreSQL, Redis, сервисы) |
| GET | `/api/config` | — | Режим агента (`llm`/`offline`) и feature flags |
| GET | `/api/auth/demo-users` | — | Тестовые учётные записи |
| POST | `/api/auth/login` `{login,password}` | — | Вход через mock-SSO |
| POST | `/api/auth/logout` | все | Выход |
| GET | `/api/auth/teams` | — | Отделы и их руководители |
| POST | `/api/auth/register` `{name,email,password}` | — | Регистрация: открывает сессию с ролью `newcomer` (ждёт назначения); руководители получают уведомление. До назначения доступны только `/api/me`, `/api/events`, уведомления (иначе 403 `awaiting_assignment`) |
| GET | `/api/department` | руководитель | Сотрудники отдела, отделы для перевода, новые сотрудники (`newcomers`) и наставники отдела |
| POST | `/api/department/{user}/assign` `{role,mentorId?}` | руководитель | Принять нового сотрудника в свой отдел: роль `intern`, `engineer` или `mentor`, наставник стажёра (по умолчанию — наставник отдела) |
| POST | `/api/department/{user}/transfer` `{team}` | руководитель | Перевод в другой отдел: новый руководитель, наставник стажёра, перенос заявок |
| GET | `/api/me` | все | Текущий пользователь, права (`perms`: `kb`, `approvals`, `programs`) и идущий тест `activeQuiz` |
| GET | `/api/messages` | все | История диалога и действия |
| POST | `/api/messages` `{text}` | все | `202`: сообщение и ответ в статусе `pending` |
| POST | `/api/messages/{id}/feedback` `{helpful}` | все | Оценка ответа |
| GET | `/api/actions` | все | Мои действия |
| POST | `/api/actions/{id}/confirm` `{edits}` | владелец | Отправить черновик в сервис |
| POST | `/api/actions/{id}/cancel` | владелец | Отменить черновик |
| GET | `/api/plan` | все | Мой план адаптации |
| PATCH | `/api/plan/steps/{step}` `{status,note,internId?}` | стажёр / наставник / руководитель | Статус шага |
| GET | `/api/goals?internId=` | стажёр (свои), наставник, руководитель, HR BP | Цели стажировки |
| POST | `/api/goals` `{internId,title,metric,due,tasks:[{title,stepId?}]}` | руководитель, наставник стажёра | Поставить цель из задач (1–30); `stepId` — задача-шаг плана |
| PATCH | `/api/goals/{id}` `{note}` | стажёр, руководитель, наставник | Комментарий к цели. Прогресс и статус считаются по закрытым задачам |
| POST | `/api/goals/{id}/tasks` `{title,stepId?}` | руководитель, наставник стажёра | Добавить задачу в цель |
| PATCH | `/api/goals/{id}/tasks/{task}` `{done}` | стажёр, руководитель, наставник | Закрыть или вернуть задачу; задача-шаг закрывается только вместе с шагом плана (400) |
| POST | `/api/cron/reminders` | ИИ-крон (`X-Cron-Token`) | Утренние сводки и напоминания о зависших заявках и вопросах |
| GET | `/api/team` | наставник, руководитель, HR BP | Мои стажёры и прогресс |
| GET | `/api/dashboard` | наставник, руководитель, HR BP | Кабинет: стажёры с отметками (`alerts`, просрочки, заявки на решение и время ожидания, наставник), заявки на решение (`waitingHours`), свои напоминания, число новых сотрудников |
| GET | `/api/team/{intern}/requests` | наставник, руководитель, HR BP | Заявки стажёра: что просил, статус, комментарий, `awaitingMe` |
| GET / POST | `/api/reminders` `{text,link?,inHours? или at?}` | все, кроме новичков | Свои напоминания; приходят уведомлением в срок (проверка раз в минуту) |
| DELETE | `/api/reminders/{id}` | автор | Удалить напоминание |
| GET | `/api/team/{intern}/plan` | наставник, руководитель, HR BP | План стажёра |
| GET | `/api/approvals` | руководитель, наставник, инженер | Заявки, где пользователь — согласующий (руководитель, владелец проекта, DevOps) |
| POST | `/api/approvals/{kind}/{id}` `{approve,comment}` | согласующий | Решение (сервис проверяет, что согласующий именно он) |
| GET | `/api/inbox` | наставник | Вопросы стажёров |
| POST | `/api/inbox/{id}/answer` `{answer,addToFaq}` | наставник | Ответ |
| GET | `/api/questions` | стажёр | Мои вопросы наставнику |
| GET | `/api/notifications`, POST `/api/notifications/read` | все | Уведомления |
| GET | `/api/insights` | наставник, руководитель, HR BP | Метрики пользы |
| GET | `/api/kb?q=`, `/api/kb/{id}` | все | Инструкции с учётом прав, папки, поиск по названию, папке, тегам и тексту |
| POST / PUT / DELETE | `/api/kb`, `/api/kb/{id}` `{title,folder,audience,tags,body}` | руководитель, право `kb` | Инструкция команды (встроенные не меняются) |
| POST / DELETE | `/api/kb/folders` `{path}` / `?path=` | руководитель, право `kb` | Папка (вложенность через «/»); удаляется только пустая |
| GET | `/api/quizzes`, `/api/quizzes/{id}` | все | Тесты с результатом пользователя; тест без правильных ответов |
| POST | `/api/quizzes/{id}/start` | все | Начать тест: на 30 минут закрывает чат и инструкции (`/api/messages`, `/api/kb*` отвечают 423 `quiz_in_progress`) |
| DELETE | `/api/quizzes/active` | все | Прервать тест без попытки |
| GET | `/api/quizzes/{id}/edit` | руководитель, право `kb` | Тест с ответами для редактора |
| POST / PUT / DELETE | `/api/quizzes`, `/api/quizzes/{id}` | руководитель, право `kb` | Тест команды |
| POST | `/api/quizzes/{id}/submit` `{answers}` | все | Проверка, разбор, закрытие шагов плана, уведомление наставника. Стажёру по ошибкам — не правильный ответ (`correct: -1`), а инструкция `docId`/`docTitle`; снимает блокировку |
| GET | `/api/quiz-results?internId=` | стажёр (свои), наставник, руководитель, HR BP | Попытки с разбором: ответ на каждый вопрос, верно или нет, правильный вариант |
| GET | `/api/quiz-results/team` | наставник, руководитель, HR BP | Попытки всех своих стажёров (сводка «стажёр × тест») |
| GET / POST / PUT / DELETE | `/api/templates`, `/api/templates/{id}` | руководитель, наставник | Шаблоны обучения: встроенные и команды |
| GET / PUT | `/api/team/{intern}/program` `{steps}` | наставник и руководитель стажёра (чтение — и HR BP) | Программа стажёра, настройка под него |
| POST | `/api/team/{intern}/program/assign` `{templateId}` | наставник и руководитель стажёра | Назначить шаблон |
| GET / POST | `/api/rights`, `{userId,perm,enabled}` | руководитель | Права сотрудников команды: `kb`, `approvals` |
| GET | `/api/events` | все | Поток обновлений (SSE) |
| GET | `/metrics` | — | Prometheus-метрики |

## MCP (corp-mock, `POST http://localhost:8090/mcp`)
JSON-RPC 2.0: `initialize`, `tools/list`, `tools/call`. Делегирование согласований: `delegation_set {user_id, enabled}` (руководитель), `delegation_list`. Заголовок `Authorization: Bearer <токен из /sso/token>`. Имитация сбоя: `POST /admin/outage {"enabled":true}` с заголовком `X-Admin-Token`.
