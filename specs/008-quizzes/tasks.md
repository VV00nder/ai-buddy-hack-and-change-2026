# Задачи 008
- [x] Модель теста, проверка и разбор, встроенные тесты — `backend/internal/quiz`
- [x] Хранение тестов и попыток — `store` (`records`, миграция `003_docs.sql`)
- [x] Сервис: список с результатами, прохождение, закрытие шагов плана, уведомление наставника, создание и правка — `app/quizzes.go`
- [x] API `/api/quizzes`, `/api/quizzes/{id}/submit`, `/api/quiz-results` — `api.go`
- [x] Агент: намерение «проверь меня», подбор теста по теме и этапу, инструменты `quiz_list` и `quiz_offer` — `agent/learning.go`, `llm_agent.go`
- [x] Шаги-тесты в шаблонах — `onboarding/templates/*.json`
- [x] Интерфейс: вкладка «Тесты», прохождение, разбор, редактор; «Проверь себя» в инструкции; кнопка в чате; результаты в карточке стажёра — `Quizzes.tsx`, `Knowledge.tsx`, `Chat.tsx`, `Programs.tsx`
