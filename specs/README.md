# Спецификации (SDD)

Разработка велась от спецификаций (Spec-Driven Development) в структуре [spec-kit](https://github.com/github/spec-kit):

1. **Конституция** — `.specify/memory/constitution.md`: принципы, которые нельзя нарушать.
2. **Спецификация фичи** — `specs/NNN-*/spec.md`: пользовательские истории, критерии приёмки в формате «Дано / Когда / Тогда», требования.
3. **Технический план** — `specs/plan.md`: архитектура и решения, общие для всех фич.
4. **Задачи** — `specs/NNN-*/tasks.md`: разбиение на задачи со ссылками на код и тесты.
5. **Поведение агента** — `specs/agent/agent-spec.md`: роли, инструменты, ограничения, промпт и проверочный набор. По нему разрабатывались AI-функции.

| Фича | Спецификация | Код | Тесты |
|---|---|---|---|
| Ответы о команде и инструментах | [001](001-qa-knowledge/spec.md) | `backend/internal/knowledge`, `agent/offline.go` | `knowledge_test.go`, `agent_test.go: TestOfflineQA` |
| План адаптации и следующий шаг | [002](002-onboarding-plan/spec.md) | `backend/internal/onboarding`, `app/plan.go`, `web/src/components/PlanView.tsx` | `onboarding_test.go`, `api_test.go: TestPlanSteps`, `TestAccessAutoCompletesPlan` |
| Заявка на отгул | [003](003-absence-request/spec.md) | `corp/absence.go`, `agent/actions.go`, `app/actions.go` | `corp_test.go: TestAbsenceRules`, `api_test.go: TestInternDayOffEndToEnd` |
| Заявка на доступ | [004](004-access-request/spec.md) | `corp/access.go`, `agent/offline.go` | `corp_test.go: TestAccess`, `agent_test.go: TestOfflineAccessScenario` |
| Вопрос наставнику и база ответов | [005](005-mentor-escalation/spec.md) | `app/team.go`, `web/src/components/Inbox.tsx` | `api_test.go: TestMentorQuestionAndFAQ` |
| Поведение агента | [agent](agent/agent-spec.md) | `backend/internal/agent` | `agent_test.go` (офлайн и LLM-режим) |

Сквозной сценарий всех фич — `e2e/demo.mjs` (12 проверок, он же записывает видео).
