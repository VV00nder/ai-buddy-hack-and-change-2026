# HTTP API

Все ответы — JSON. Изменяющие запросы требуют заголовок `X-Requested-With: ai-buddy`. Авторизация — cookie `buddy_session` после входа.

| Метод | Путь | Роль | Описание |
|---|---|---|---|
| GET | `/api/health`, `/api/ready` | — | Живость и готовность (PostgreSQL, Redis, сервисы) |
| GET | `/api/config` | — | Режим агента (`llm`/`offline`) и feature flags |
| GET | `/api/auth/demo-users` | — | Тестовые учётные записи |
| POST | `/api/auth/login` `{login,password}` | — | Вход через mock-SSO |
| POST | `/api/auth/logout` | все | Выход |
| GET | `/api/me` | все | Текущий пользователь |
| GET | `/api/messages` | все | История диалога и действия |
| POST | `/api/messages` `{text}` | все | `202`: сообщение и ответ в статусе `pending` |
| POST | `/api/messages/{id}/feedback` `{helpful}` | все | Оценка ответа |
| GET | `/api/actions` | все | Мои действия |
| POST | `/api/actions/{id}/confirm` `{edits}` | владелец | Отправить черновик в сервис |
| POST | `/api/actions/{id}/cancel` | владелец | Отменить черновик |
| GET | `/api/plan` | все | Мой план адаптации |
| PATCH | `/api/plan/steps/{step}` `{status,note,internId?}` | стажёр / наставник / руководитель | Статус шага |
| GET | `/api/team` | наставник, руководитель, HR BP | Мои стажёры и прогресс |
| GET | `/api/team/{intern}/plan` | наставник, руководитель, HR BP | План стажёра |
| GET | `/api/approvals` | руководитель | Заявки на согласование |
| POST | `/api/approvals/{kind}/{id}` `{approve,comment}` | руководитель | Решение |
| GET | `/api/inbox` | наставник | Вопросы стажёров |
| POST | `/api/inbox/{id}/answer` `{answer,addToFaq}` | наставник | Ответ |
| GET | `/api/questions` | стажёр | Мои вопросы наставнику |
| GET | `/api/notifications`, POST `/api/notifications/read` | все | Уведомления |
| GET | `/api/insights` | наставник, руководитель, HR BP | Метрики пользы |
| GET | `/api/kb`, `/api/kb/{id}` | все | Инструкции с учётом прав |
| GET | `/api/events` | все | Поток обновлений (SSE) |
| GET | `/metrics` | — | Prometheus-метрики |

## MCP (corp-mock, `POST http://localhost:8090/mcp`)
JSON-RPC 2.0: `initialize`, `tools/list`, `tools/call`. Заголовок `Authorization: Bearer <токен из /sso/token>`. Имитация сбоя: `POST /admin/outage {"enabled":true}` с заголовком `X-Admin-Token`.
