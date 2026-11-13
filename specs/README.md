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
| Цели стажировки (трекер прогресса) | [006](006-goals/spec.md) | `app/goals.go`, `web/src/components/Goals.tsx` | `api_test.go: TestGoals`, `agent_test.go: TestGoalsIntent` |
| Проактивные напоминания (ИИ-крон) | [007](007-proactive/spec.md) | `app/cron.go` | `api_test.go: TestCronReminders` |
| Тесты после тем и блоков | [008](008-quizzes/spec.md) | `backend/internal/quiz`, `app/quizzes.go`, `web/src/components/Quizzes.tsx` | `quiz_test.go`, `learning_test.go: TestQuizzes` |
| Шаблоны обучения и программа под стажёра | [009](009-programs/spec.md) | `onboarding`, `app/programs.go`, `web/src/components/Programs.tsx` | `onboarding_test.go: TestTemplates`, `learning_test.go: TestTemplatesAndPrograms` |
| Инструкции: папки, поиск, редактирование | [010](010-kb-editing/spec.md) | `knowledge`, `app/kbadmin.go`, `web/src/components/Knowledge.tsx` | `learning_test.go: TestRightsAndKBEditing` |
| Права от руководителя | [011](011-delegation/spec.md) | `corp/delegation.go`, `app/rights.go`, `web/src/components/Rights.tsx` | `corp_test.go: TestDeputies`, `learning_test.go: TestApprovalDelegation` |
| Отметка шагов из чата | [002](002-onboarding-plan/spec.md) | `agent/learning.go`, `TodayPanel.tsx` | `learning_test.go: TestChatMarksStepsAndOffersQuiz` |
| Поведение агента | [agent](agent/agent-spec.md) | `backend/internal/agent` | `agent_test.go` (офлайн и LLM-режим) |

Сквозной сценарий всех фич — `e2e/demo.mjs` (он же записывает видео).

Спецификации 008–011 и отметка шагов из чата добавлены 13.11.2026 по итогам прогона прототипа командой.

Спецификации 004, 006 и 007 уточнены по ответам экспертов Авито в чате трека (10.11.2026): состав заявки на доступ и согласующие по типу ресурса, цели стажировки, имитация ИИ-крона.
