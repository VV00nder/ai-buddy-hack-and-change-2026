# HTTP API

Все ответы — JSON. Изменяющие запросы требуют заголовок `X-Requested-With: ai-buddy`. Авторизация — cookie `buddy_session` после входа.

| Метод | Путь | Роль | Описание |
|---|---|---|---|
| GET | `/api/health`, `/api/ready` | — | Живость и готовность (PostgreSQL, Redis, сервисы) |
| GET | `/api/config` | — | Режим агента (`llm`/`offline`) и feature flags |
| GET | `/api/auth/demo-users` | — | Тестовые учётные записи |
| POST | `/api/auth/login` `{login,password}` | — | Вход через mock-SSO |
| POST | `/api/auth/logout` | все | Выход |
| GET | `/api/me` | все | Текущий пользователь и права (`perms`: `kb`, `approvals`, `programs`) |
| GET | `/api/messages` | все | История диалога и действия |
| POST | `/api/messages` `{text}` | все | `202`: сообщение и ответ в статусе `pending` |
| POST | `/api/messages/{id}/feedback` `{helpful}` | все | Оценка ответа |
| GET | `/api/actions` | все | Мои действия |
| POST | `/api/actions/{id}/confirm` `{edits}` | владелец | Отправить черновик в сервис |
| POST | `/api/actions/{id}/cancel` | владелец | Отменить черновик |
| GET | `/api/plan` | все | Мой план адаптации |
| PATCH | `/api/plan/steps/{step}` `{status,note,internId?}` | стажёр / наставник / руководитель | Статус шага |
| GET | `/api/goals?internId=` | стажёр (свои), наставник, руководитель, HR BP | Цели стажировки |
| POST | `/api/goals` `{internId,title,metric,due}` | руководитель, наставник стажёра | Поставить цель |
| PATCH | `/api/goals/{id}` `{status?,progress?,note?}` | стажёр, руководитель, наставник | Прогресс и статус цели |
| POST | `/api/cron/reminders` | ИИ-крон (`X-Cron-Token`) | Утренние сводки и напоминания о зависших заявках и вопросах |
| GET | `/api/team` | наставник, руководитель, HR BP | Мои стажёры и прогресс |
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
| GET | `/api/quizzes/{id}/edit` | руководитель, право `kb` | Тест с ответами для редактора |
| POST / PUT / DELETE | `/api/quizzes`, `/api/quizzes/{id}` | руководитель, право `kb` | Тест команды |
| POST | `/api/quizzes/{id}/submit` `{answers}` | все | Проверка, разбор, закрытие шагов плана, уведомление наставника |
| GET | `/api/quiz-results?internId=` | стажёр (свои), наставник, руководитель, HR BP | Попытки прохождения тестов |
| GET / POST / PUT / DELETE | `/api/templates`, `/api/templates/{id}` | руководитель, наставник | Шаблоны обучения: встроенные и команды |
| GET / PUT | `/api/team/{intern}/program` `{steps}` | наставник и руководитель стажёра (чтение — и HR BP) | Программа стажёра, настройка под него |
| POST | `/api/team/{intern}/program/assign` `{templateId}` | наставник и руководитель стажёра | Назначить шаблон |
| GET / POST | `/api/rights`, `{userId,perm,enabled}` | руководитель | Права сотрудников команды: `kb`, `approvals` |
| GET | `/api/events` | все | Поток обновлений (SSE) |
| GET | `/metrics` | — | Prometheus-метрики |

## MCP (corp-mock, `POST http://localhost:8090/mcp`)
JSON-RPC 2.0: `initialize`, `tools/list`, `tools/call`. Делегирование согласований: `delegation_set {user_id, enabled}` (руководитель), `delegation_list`. Заголовок `Authorization: Bearer <токен из /sso/token>`. Имитация сбоя: `POST /admin/outage {"enabled":true}` с заголовком `X-Admin-Token`.
