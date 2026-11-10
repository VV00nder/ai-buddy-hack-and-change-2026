# Задачи 001
- [x] Формат документа: front matter `id, title, audience, team, tags` + разделы `##` — `internal/knowledge/docs/*.md`
- [x] Индекс BM25 и стемминг — `internal/knowledge/knowledge.go`
- [x] Права `CanRead` (роль + команда) — `knowledge.go`
- [x] Метрика покрытия запроса (`Coverage`) и порог уверенности — `agent/offline.go`
- [x] Инструмент `kb_search` для LLM — `agent/llm_agent.go`
- [x] Ответ со ссылками на источники и переход к инструкции — `web/src/components/Chat.tsx`, `Knowledge.tsx`
- [x] Тесты релевантности и прав — `knowledge_test.go`
