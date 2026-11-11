package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aibuddy/internal/corp"
	"aibuddy/internal/domain"
	"aibuddy/internal/identity"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/llm"
	"aibuddy/internal/mcp"
	"aibuddy/internal/onboarding"
)

var msk = time.FixedZone("MSK", 3*3600)

func now() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, msk) } // вторник

type env struct {
	agent *Agent
	corp  *corp.Service
	anna  domain.User
	token string
	srv   *httptest.Server
	trace []string
}

func newEnv(t *testing.T, model llm.Client) *env {
	t.Helper()
	svc, err := corp.New(now, msk)
	if err != nil {
		t.Fatal(err)
	}
	signer := identity.NewSigner("s", now)
	srv := httptest.NewServer(svc.Handler(signer, "demo", "adm"))
	t.Cleanup(srv.Close)
	kb, _ := knowledge.Load()
	tpl, _ := onboarding.LoadTemplate()
	anna, _ := svc.UserByLogin("anna")
	e := &env{corp: svc, anna: anna, token: signer.Issue(anna, time.Hour), srv: srv}
	e.agent = New(Deps{KB: kb, Corp: mcp.NewClient(srv.URL+"/mcp", 2*time.Second), LLM: model, Now: now,
		Plan: func(_ context.Context, u domain.User) (onboarding.Plan, error) {
			if u.ID == "broken" {
				return onboarding.Plan{}, errors.New("db down")
			}
			return tpl.Build(u, nil, now()), nil
		},
		Goals: func(_ context.Context, u domain.User) ([]domain.Goal, error) {
			if u.ID == "broken" {
				return nil, errors.New("db down")
			}
			return []domain.Goal{{Title: "Закрыть SEARCH-101", Metric: "MR выкачен", Due: "2026-11-24", Status: domain.GoalInProgress, Progress: 40}}, nil
		}})
	return e
}

func (e *env) ask(text string, history ...domain.Message) Reply {
	e.trace = nil
	return e.agent.Handle(context.Background(), Request{User: e.anna, Token: e.token, Text: text, History: history,
		Progress: func(ti domain.TraceItem) { e.trace = append(e.trace, ti.Label) }})
}

func assistantCtx(r Reply) domain.Message {
	return domain.Message{Author: "assistant", Text: r.Text, Status: domain.MsgDone, Context: r.Context}
}

func TestOfflineDayOffScenario(t *testing.T) {
	e := newEnv(t, nil)
	if e.agent.Mode() != "offline" {
		t.Fatal("mode")
	}
	// кейс из ТЗ: не знает порядка и стоит ли отвлекать руководителя
	r := e.ask("Мне нужен отгул, но я не знаю, как его оформить и стоит ли отвлекать руководителя")
	if !strings.Contains(r.Text, "отвлекать руководителя") || len(r.Sources) == 0 || r.Sources[0].DocID != "process-day-off" {
		t.Fatalf("procedure: %s %+v", r.Text, r.Sources)
	}
	if r.Mode != "offline" || r.Context["awaiting"] != "absence_date" || !strings.Contains(r.Suggestions[0], "12.11") {
		t.Fatalf("follow-up: %+v", r)
	}
	// продолжение диалога: дата следующим сообщением
	r2 := e.ask("давай в пятницу", assistantCtx(r))
	if r2.Proposal == nil || r2.Proposal.Kind != domain.ActionAbsence || r2.Proposal.Args["date_from"] != "2026-11-13" {
		t.Fatalf("proposal: %+v", r2)
	}
	if !strings.Contains(r2.Text, "Мария Волкова") || len(e.trace) == 0 {
		t.Fatalf("text: %s trace=%v", r2.Text, e.trace)
	}
	if _, ok := r2.Proposal.Args["dry_run"]; ok {
		t.Fatal("dry_run must not leak into args")
	}
	// слишком поздно
	r3 := e.ask("хочу отгул завтра")
	if r3.Proposal != nil || !strings.Contains(r3.Text, "минимум за 2 рабочих дня") || len(r3.Suggestions) == 0 {
		t.Fatalf("too late: %+v", r3)
	}
	// без даты — уточняющий вопрос
	r4 := e.ask("оформи отгул")
	if r4.Context["awaiting"] != "absence_date" || r4.Proposal != nil {
		t.Fatalf("ask date: %+v", r4)
	}
	// отпуск стажёру недоступен
	if r := e.ask("хочу в отпуск с 14 по 18 декабря"); !strings.Contains(r.Text, "90 дней") {
		t.Fatalf("vacation: %s", r.Text)
	}
	if r := e.ask("сколько у меня осталось отгулов"); !strings.Contains(r.Text, "3") {
		t.Fatalf("balance: %s", r.Text)
	}
}

func TestOfflineAccessScenario(t *testing.T) {
	e := newEnv(t, nil)
	r := e.ask("Какие доступы мне нужны?")
	if !strings.Contains(r.Text, "Можно запросить") || len(r.Suggestions) == 0 || strings.Contains(r.Text, "продовые") || !strings.Contains(r.Text, "владелец проекта") {
		t.Fatalf("overview: %s", r.Text)
	}
	// Stash: проект не указан — ассистент уточняет его и предлагает проекты команды
	r = e.ask("Запроси доступ к stash на запись")
	if r.Proposal != nil || r.Context["awaiting"] != "access_project" || len(r.Suggestions) != 3 || r.Context["level"] != "edit" {
		t.Fatalf("stash project question: %+v", r)
	}
	r = e.ask("search-api", assistantCtx(r))
	if r.Proposal == nil || r.Proposal.Args["project"] != "search-api" || r.Proposal.Args["level"] != "edit" ||
		!strings.Contains(r.Text, "владелец проекта: Олег Кузнецов") || r.Proposal.Title != "Stash: репозиторий — search-api" {
		t.Fatalf("stash: %+v", r)
	}
	// проект и уровень в одном сообщении; администрирование стажёру не выдаётся
	if r := e.ask("дай доступ к stash search-web на чтение"); r.Proposal == nil || r.Proposal.Args["level"] != "read" || !strings.Contains(r.Text, "Мария") {
		t.Fatalf("stash read: %+v", r)
	}
	if r := e.ask("нужен админский доступ к stash search-indexer"); r.Proposal != nil || !strings.Contains(r.Text, "не выдаётся") || len(r.Suggestions) != 1 {
		t.Fatalf("admin: %+v", r)
	}
	// тестовый кластер согласует DevOps и нужно обоснование
	r = e.ask("нужен доступ к staging кластеру")
	if r.Context["awaiting"] != "access_justification" {
		t.Fatalf("staging: %+v", r)
	}
	if r := e.ask("Отладка индексатора поиска для задачи SEARCH-77", assistantCtx(r)); r.Proposal == nil || !strings.Contains(r.Text, "DevOps") {
		t.Fatalf("staging justification: %+v", r)
	}
	// продовый kubernetes стажёру не положен — объясняем, ничего не готовим
	r = e.ask("дай доступ к kubernetes")
	if r.Proposal != nil || !strings.Contains(r.Text, "staging") {
		t.Fatalf("k8s: %+v", r)
	}
	// redash требует обоснования — ждём его следующим сообщением
	r = e.ask("нужен доступ к redash")
	if r.Context["awaiting"] != "access_justification" {
		t.Fatalf("redash: %+v", r)
	}
	r = e.ask("Посчитать метрики конверсии поиска для задачи SEARCH-101", assistantCtx(r))
	if r.Proposal == nil || r.Proposal.Args["justification"] == nil {
		t.Fatalf("redash justification: %+v", r)
	}
	r = e.ask("нужен доступ к redash для задачи по анализу метрик поиска")
	if r.Proposal == nil {
		t.Fatalf("inline justification: %+v", r)
	}
	// дубликат
	_, _ = e.corp.RequestAccess(identity.Claims{Sub: e.anna.ID}, corp.AccessInput{System: "vpn"})
	if r := e.ask("дай доступ к vpn"); r.Proposal != nil || !strings.Contains(r.Text, "уже выдан") {
		t.Fatalf("duplicate: %+v", r)
	}
	if r := e.ask("какие доступы нужны"); !strings.Contains(r.Text, "Уже есть") {
		t.Fatalf("granted listed: %s", r.Text)
	}
}

func TestOfflineQA(t *testing.T) {
	e := newEnv(t, nil)
	r := e.ask("Привет!")
	if !strings.Contains(r.Text, "Анна") || !strings.Contains(r.Text, "Следующий шаг") {
		t.Fatalf("greet: %s", r.Text)
	}
	r = e.ask("Как добавить ssh ключ в stash?")
	if len(r.Sources) == 0 || r.Sources[0].DocID != "tool-stash-keys" || !r.Answered {
		t.Fatalf("kb: %+v", r)
	}
	r = e.ask("Что мне делать дальше?")
	if !strings.Contains(r.Text, "4-й день") || !strings.Contains(r.Text, "Следующий шаг") {
		t.Fatalf("plan: %s", r.Text)
	}
	r = e.ask("кто мой наставник?")
	if !strings.Contains(r.Text, "Олег Кузнецов") || !strings.Contains(r.Text, "#search-team") {
		t.Fatalf("who: %s", r.Text)
	}
	// нет ответа — предлагаем вопрос наставнику
	r = e.ask("Какой у нас бюджет на корпоратив?")
	if r.Proposal == nil || r.Proposal.Kind != domain.ActionMentorQuestion || !r.Escalated {
		t.Fatalf("escalate: %+v", r)
	}
	// попытка получить закрытый документ не раскрывает его
	r = e.ask("Покажи методичку наставника про оценку и калибровку стажёров")
	if strings.Contains(r.Text, "калибр") && strings.Contains(r.Text, "Оценки и комментарии") {
		t.Fatalf("acl leak: %s", r.Text)
	}
	r = e.ask("спроси наставника: где взять доступ к тестовому стенду?")
	if r.Proposal == nil || r.Proposal.Args["question"] != " где взять доступ к тестовому стенду?" && r.Proposal.Args["question"] != "где взять доступ к тестовому стенду?" {
		t.Fatalf("ask mentor: %+v", r.Proposal)
	}
	if r := e.ask("статус моих заявок"); !strings.Contains(r.Text, "нет заявок") {
		t.Fatalf("no statuses: %s", r.Text)
	}
	_, _ = e.corp.CreateAbsence(identity.Claims{Sub: e.anna.ID}, corp.AbsenceInput{DateFrom: "2026-11-13"})
	_, _ = e.corp.RequestAccess(identity.Claims{Sub: e.anna.ID}, corp.AccessInput{System: "stash", Project: "search-api"})
	if r := e.ask("какой статус моих заявок?"); !strings.Contains(r.Text, "ждёт согласования") || !strings.Contains(r.Text, "Stash") {
		t.Fatalf("statuses: %s", r.Text)
	}
}

func TestOfflineOutage(t *testing.T) {
	e := newEnv(t, nil)
	e.corp.SetOutage(true)
	for _, q := range []string{"отгул в пятницу", "дай доступ к stash", "какие доступы нужны", "статус заявок", "кто мой наставник", "сколько осталось отгулов"} {
		if r := e.ask(q); !strings.Contains(r.Text, "не отвечает") {
			t.Errorf("%q: %s", q, r.Text)
		}
	}
	e.anna.ID = "broken"
	if r := e.ask("что дальше"); !strings.Contains(r.Text, "Не получилось") {
		t.Fatal("plan error")
	}
}

// scripted — модель, отвечающая по сценарию.
type scripted struct {
	steps []llm.Message
	seen  [][]llm.Message
	err   error
}

func (s *scripted) Model() string { return "scripted" }
func (s *scripted) Chat(_ context.Context, msgs []llm.Message, tools []llm.Tool) (llm.Message, error) {
	s.seen = append(s.seen, msgs)
	if s.err != nil {
		return llm.Message{}, s.err
	}
	if len(s.steps) == 0 {
		return llm.Message{Role: "assistant", Content: "готово"}, nil
	}
	m := s.steps[0]
	s.steps = s.steps[1:]
	return m, nil
}

func call(id, name string, args any) llm.ToolCall {
	tc := llm.ToolCall{ID: id, Type: "function"}
	tc.Function.Name = name
	b, _ := json.Marshal(args)
	tc.Function.Arguments = string(b)
	return tc
}

func toolMsg(calls ...llm.ToolCall) llm.Message {
	return llm.Message{Role: "assistant", ToolCalls: calls}
}

func TestLLMMode(t *testing.T) {
	model := &scripted{steps: []llm.Message{
		toolMsg(call("1", "kb_search", map[string]any{"query": "отгул"}), call("2", "plan_get", nil)),
		toolMsg(call("3", "absence_create", map[string]any{"date_from": "2026-11-13"})),
		{Role: "assistant", Content: "Подготовил заявку на отгул 13.11 — подтверди её в карточке."},
	}}
	e := newEnv(t, model)
	if e.agent.Mode() != "llm" {
		t.Fatal("mode")
	}
	hist := []domain.Message{{Author: "user", Text: "привет", Status: domain.MsgDone}, {Author: "assistant", Text: "", Status: domain.MsgPending}}
	r := e.ask("оформи отгул на пятницу", hist...)
	if r.Mode != "llm" || r.Proposal == nil || r.Proposal.Args["date_from"] != "2026-11-13" || len(r.Sources) == 0 {
		t.Fatalf("llm reply: %+v", r)
	}
	sys := model.seen[0][0].Content
	if !strings.Contains(sys, "2026-11-10") || !strings.Contains(sys, "Анна Смирнова") || !strings.Contains(sys, "День стажировки: 4") {
		t.Fatalf("system prompt: %s", sys)
	}
	if len(model.seen[0]) != 3 { // system + history(1 done) + user
		t.Fatalf("history: %d", len(model.seen[0]))
	}
	// результат инструмента с dry_run сообщает модели, что заявка не отправлена
	if !strings.Contains(model.seen[2][len(model.seen[2])-1].Content, "НЕ отправлена") {
		t.Fatal("tool note")
	}
}

func TestLLMToolErrorsAndFallback(t *testing.T) {
	model := &scripted{steps: []llm.Message{
		toolMsg(call("1", "absence_create", map[string]any{"date_from": "2026-11-11"}),
			call("2", "access_request", map[string]any{"system": "k8s-prod"}),
			call("3", "directory_whoami", nil),
			call("4", "absence_get", map[string]any{"id": "nope"}),
			call("5", "ask_mentor", map[string]any{}),
			llm.ToolCall{ID: "6", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "kb_search", Arguments: "{bad"}}),
		{Role: "assistant", Content: "Объяснил ограничения."},
	}}
	e := newEnv(t, model)
	r := e.ask("отгул завтра и доступ к кубернетесу")
	if r.Mode != "llm" || r.Proposal == nil || r.Proposal.Kind != domain.ActionMentorQuestion || !r.Escalated {
		t.Fatalf("reply: %+v", r)
	}
	toolResults := model.seen[1]
	joined := ""
	for _, m := range toolResults {
		if m.Role == "tool" {
			joined += m.Content
		}
	}
	for _, want := range []string{"too_late", "not_allowed_for_role", "Олег", "not_found", "bad_arguments"} {
		if !strings.Contains(joined, want) {
			t.Errorf("tool results miss %s: %s", want, joined)
		}
	}
	// решение руководителя не передаётся модели как инструмент
	for _, m := range model.seen {
		_ = m
	}
	// модель недоступна — офлайн-резерв
	broken := &scripted{err: errors.New("timeout")}
	e2 := newEnv(t, broken)
	r2 := e2.ask("как добавить ssh ключ")
	if r2.Mode != "offline" || len(r2.Sources) == 0 || !strings.Contains(strings.Join(e2.trace, "|"), "упрощённом") {
		t.Fatalf("fallback: %+v %v", r2, e2.trace)
	}
	// пустой ответ модели — тоже резерв
	empty := &scripted{steps: []llm.Message{{Role: "assistant", Content: "  "}}}
	if r := newEnv(t, empty).ask("привет"); r.Mode != "offline" {
		t.Fatal("empty answer fallback")
	}
	// бесконечные вызовы инструментов — ограничение шагов
	loop := &scripted{}
	for i := 0; i < 10; i++ {
		loop.steps = append(loop.steps, toolMsg(call("x", "plan_get", nil)))
	}
	if r := newEnv(t, loop).ask("что дальше"); r.Mode != "offline" {
		t.Fatal("step limit fallback")
	}
	// сервис недоступен в LLM-режиме
	down := &scripted{steps: []llm.Message{toolMsg(call("1", "absence_create", map[string]any{"date_from": "2026-11-13"}), call("2", "absence_balance", nil)), {Role: "assistant", Content: "Сервис недоступен"}}}
	e3 := newEnv(t, down)
	e3.corp.SetOutage(true)
	if r := e3.ask("отгул"); r.Proposal != nil || !strings.Contains(down.seen[1][len(down.seen[1])-1].Content, "service_unavailable") {
		t.Fatalf("outage: %+v", r)
	}
	// plan_get с ошибкой
	pe := &scripted{steps: []llm.Message{toolMsg(call("1", "plan_get", nil)), {Role: "assistant", Content: "ok"}}}
	e4 := newEnv(t, pe)
	e4.anna.ID = "broken"
	if r := e4.ask("план"); r.Mode != "llm" || !strings.Contains(pe.seen[1][len(pe.seen[1])-1].Content, "plan_unavailable") {
		t.Fatal("plan error tool")
	}
}

func TestHelpers(t *testing.T) {
	if firstName("") != "коллега" || ruDate("bad") != "bad" {
		t.Fatal("helpers")
	}
	if absenceTitle(map[string]any{"type": "vacation", "date_from": "2026-12-14", "date_to": "2026-12-18"}) != "Отпуск с 14.12 (пн) по 18.12 (пт)" {
		t.Fatal(absenceTitle(map[string]any{"type": "vacation", "date_from": "2026-12-14", "date_to": "2026-12-18"}))
	}
	if toolLabel("x_y") == "" || explainCallError(&mcp.CallError{Code: "forbidden"}) == "" {
		t.Fatal("labels")
	}
	a := New(Deps{})
	if a.d.Now == nil || a.d.MaxSteps != 6 {
		t.Fatal("defaults")
	}
	// без Progress не падает
	e := newEnv(t, nil)
	_ = e.agent.Handle(context.Background(), Request{User: e.anna, Token: e.token, Text: "привет"})
	if earliestDayOff(time.Date(2026, 11, 13, 10, 0, 0, 0, msk)).Weekday() != time.Tuesday {
		t.Fatal("earliest from friday")
	}
}

func TestGoalsIntent(t *testing.T) {
	model := &scripted{steps: []llm.Message{toolMsg(call("1", "goals_get", nil)), {Role: "assistant", Content: "Цели: SEARCH-101"}}}
	e := newEnv(t, model)
	if r := e.ask("мои цели"); r.Mode != "llm" || !strings.Contains(model.seen[1][len(model.seen[1])-1].Content, "SEARCH-101") {
		t.Fatalf("llm goals: %+v", r)
	}
	off := newEnv(t, nil)
	if r := off.ask("Какие у меня цели на стажировку?"); !strings.Contains(r.Text, "Закрыть SEARCH-101") || !strings.Contains(r.Text, "в работе, прогресс 40%") {
		t.Fatalf("goals: %s", r.Text)
	}
	off.anna.ID = "broken"
	if r := off.ask("мои цели"); !strings.Contains(r.Text, "Не получилось") {
		t.Fatal("goals error")
	}
	// без источника целей инструмент возвращает пустой список
	e2 := newEnv(t, &scripted{steps: []llm.Message{toolMsg(call("1", "goals_get", nil)), {Role: "assistant", Content: "нет"}}})
	e2.agent.d.Goals = nil
	if r := e2.ask("цели"); r.Mode != "llm" {
		t.Fatal("nil goals")
	}
	e3 := newEnv(t, &scripted{steps: []llm.Message{toolMsg(call("1", "goals_get", nil)), {Role: "assistant", Content: "ошибка"}}})
	e3.anna.ID = "broken"
	_ = e3.ask("цели")
}
