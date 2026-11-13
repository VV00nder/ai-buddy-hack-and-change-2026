package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aibuddy/internal/agent"
	"aibuddy/internal/app"
	"aibuddy/internal/corp"
	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/flags"
	"aibuddy/internal/identity"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/kv"
	"aibuddy/internal/mcp"
	"aibuddy/internal/onboarding"
	"aibuddy/internal/store"
)

var msk = time.FixedZone("MSK", 3*3600)

type testEnv struct {
	t      *testing.T
	corp   *corp.Service
	corpTS *httptest.Server
	ts     *httptest.Server
	app    *app.App
	st     *store.Memory
	clock  time.Time
}

func newTestEnv(t *testing.T, ff flags.Flags, webDir string) *testEnv {
	t.Helper()
	e := &testEnv{t: t, clock: time.Date(2026, 11, 10, 12, 0, 0, 0, msk)}
	now := func() time.Time { return e.clock }
	svc, _ := corp.New(now, msk)
	e.corp = svc
	signer := identity.NewSigner("secret", now)
	e.corpTS = httptest.NewServer(svc.Handler(signer, "buddy2026", "adm"))
	t.Cleanup(e.corpTS.Close)
	kb, _ := knowledge.Load()
	e.st = store.NewMemory()
	corpClient := mcp.NewClient(e.corpTS.URL+"/mcp", 2*time.Second)
	plan := func(ctx context.Context, u domain.User) (onboarding.Plan, error) { return e.app.PlanFor(ctx, u) }
	goals := func(ctx context.Context, u domain.User) ([]domain.Goal, error) { return e.st.Goals(ctx, u.ID) }
	ag := agent.New(agent.Deps{KB: kb, Corp: corpClient, Plan: plan, Goals: goals, Now: now,
		SetStep: func(ctx context.Context, u domain.User, step string, st domain.StepStatus) (onboarding.Plan, error) {
			return e.app.SetStep(ctx, u, u.ID, step, st, "")
		},
		Quizzes: func(ctx context.Context, u domain.User) ([]agent.QuizRef, error) { return e.app.QuizRefs(ctx, u) }})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e.app = app.New(app.Config{CorpBaseURL: e.corpTS.URL, MsgRateLimit: 30},
		app.Deps{Store: e.st, KV: kv.NewMemory(now), Corp: corpClient, KB: kb, Agent: ag, Hub: events.NewHub(), Flags: ff, Now: now, Log: log})
	e.ts = httptest.NewServer(New(e.app, kb, log, webDir, false).Handler())
	t.Cleanup(e.ts.Close)
	return e
}

func allOn() flags.Flags { return flags.FromEnv(func(string) string { return "" }) }

type client struct {
	t    *testing.T
	e    *testEnv
	hc   *http.Client
	user domain.User
}

func (e *testEnv) login(login string) *client {
	jar, _ := cookiejar.New(nil)
	c := &client{t: e.t, e: e, hc: &http.Client{Jar: jar}}
	var out struct{ User domain.User }
	if code := c.do("POST", "/api/auth/login", map[string]string{"login": login, "password": "buddy2026"}, &out); code != 200 {
		e.t.Fatalf("login %s: %d", login, code)
	}
	c.user = out.User
	return c
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.ts.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "ai-buddy")
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

type chatState struct {
	Messages []domain.Message
	Actions  []domain.Action
}

// ask отправляет сообщение и ждёт готовый ответ ассистента.
func (c *client) ask(text string) (domain.Message, *domain.Action) {
	c.t.Helper()
	var sent struct{ User, Assistant domain.Message }
	if code := c.do("POST", "/api/messages", map[string]string{"text": text}, &sent); code != 202 {
		c.t.Fatalf("send %q: %d", text, code)
	}
	if sent.Assistant.Status != domain.MsgPending {
		c.t.Fatal("assistant must start pending")
	}
	c.e.app.Wait()
	var st chatState
	c.do("GET", "/api/messages", nil, &st)
	for _, m := range st.Messages {
		if m.ID == sent.Assistant.ID {
			for i := range st.Actions {
				if st.Actions[i].ID == m.ActionID {
					return m, &st.Actions[i]
				}
			}
			return m, nil
		}
	}
	c.t.Fatal("reply not found")
	return domain.Message{}, nil
}

func TestInternDayOffEndToEnd(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	maria := e.login("maria")

	// SSE: стажёр подписан на обновления
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/events", nil)
	resp, err := anna.hc.Do(req)
	if err != nil || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("sse: %v", err)
	}
	lines := make(chan string, 100)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	msg, act := anna.ask("Хочу отгул в пятницу")
	if act == nil || act.Status != domain.ActionProposed || act.Kind != domain.ActionAbsence {
		t.Fatalf("proposal: %s %+v", msg.Text, act)
	}
	if len(msg.Trace) < 2 {
		t.Fatal("trace must show progress")
	}
	// стажёр правит причину и подтверждает
	var conf struct{ Action domain.Action }
	if code := anna.do("POST", "/api/actions/"+act.ID+"/confirm", map[string]any{"edits": map[string]any{"reason": "сессия в вузе"}}, &conf); code != 200 {
		t.Fatalf("confirm: %d", code)
	}
	if conf.Action.Status != domain.ActionSubmitted || conf.Action.ExternalID == "" || conf.Action.Args["reason"] != "сессия в вузе" {
		t.Fatalf("submitted: %+v", conf.Action)
	}
	// повторное подтверждение и отмена отправленного невозможны
	if code := anna.do("POST", "/api/actions/"+act.ID+"/confirm", map[string]any{}, nil); code != 400 {
		t.Fatalf("double confirm: %d", code)
	}
	if code := anna.do("POST", "/api/actions/"+act.ID+"/cancel", nil, nil); code != 400 {
		t.Fatalf("cancel submitted: %d", code)
	}
	// руководитель видит заявку и согласует
	var ap struct{ Approvals []app.Approval }
	maria.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 1 || ap.Approvals[0].UserName != "Анна Смирнова" {
		t.Fatalf("approvals: %+v", ap)
	}
	if code := anna.do("GET", "/api/approvals", nil, nil); code != 403 {
		t.Fatal("intern must not see approvals")
	}
	if code := maria.do("POST", "/api/approvals/absence/"+ap.Approvals[0].ExternalID, map[string]any{"approve": true, "comment": "Хорошего отдыха"}, nil); code != 200 {
		t.Fatalf("decide: %d", code)
	}
	var acts struct{ Actions []domain.Action }
	anna.do("GET", "/api/actions", nil, &acts)
	if acts.Actions[0].Status != domain.ActionApproved {
		t.Fatalf("approved: %+v", acts.Actions[0])
	}
	var notes struct{ Notifications []domain.Notification }
	anna.do("GET", "/api/notifications", nil, &notes)
	if len(notes.Notifications) < 2 || !strings.Contains(notes.Notifications[0].Text, "Хорошего отдыха") {
		t.Fatalf("notifications: %+v", notes)
	}
	anna.do("POST", "/api/notifications/read", nil, nil)
	anna.do("GET", "/api/notifications", nil, &notes)
	if !notes.Notifications[0].Read {
		t.Fatal("read")
	}
	// статус заявки виден в чате
	if m, _ := anna.ask("какой статус моих заявок?"); !strings.Contains(m.Text, "согласована") {
		t.Fatalf("status: %s", m.Text)
	}
	// SSE донёс обновления
	got := ""
	timeout := time.After(2 * time.Second)
	for !strings.Contains(got, `"type":"action"`) {
		select {
		case l := <-lines:
			got += l
		case <-timeout:
			t.Fatalf("sse events: %s", got)
		}
	}
	resp.Body.Close()
	// оценка ответа
	if code := anna.do("POST", "/api/messages/"+msg.ID+"/feedback", map[string]bool{"helpful": true}, nil); code != 200 {
		t.Fatal("feedback")
	}
	if code := anna.do("POST", "/api/messages/unknown/feedback", map[string]bool{"helpful": true}, nil); code != 404 {
		t.Fatal("feedback 404")
	}
	// метрики пользы для руководителя
	var in app.Insights
	if code := maria.do("GET", "/api/insights", nil, &in); code != 200 || in.Questions != 2 || in.ActionsSubmitted != 1 || in.HelpfulRate != 100 || in.MentorMinutesSaved == 0 {
		t.Fatalf("insights: %d %+v", code, in)
	}
	if code := anna.do("GET", "/api/insights", nil, nil); code != 403 {
		t.Fatal("intern insights")
	}
}

func TestAccessAutoCompletesPlan(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	maria := e.login("maria")
	oleg := e.login("oleg")
	_, act := anna.ask("Запроси доступ к stash search-api на запись")
	var conf struct{ Action domain.Action }
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, &conf)
	if conf.Action.Status != domain.ActionSubmitted {
		t.Fatalf("stash pending: %+v", conf.Action)
	}
	// Stash согласует владелец проекта (Олег), а не руководитель
	var ap struct{ Approvals []app.Approval }
	maria.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 0 {
		t.Fatalf("lead must not get owner's approval: %+v", ap)
	}
	oleg.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 1 || ap.Approvals[0].Title != "Доступ: Stash: репозиторий — search-api, редактирование" {
		t.Fatalf("owner approvals: %+v", ap)
	}
	if code := maria.do("POST", "/api/approvals/access/"+ap.Approvals[0].ExternalID, map[string]any{"approve": true}, nil); code != 403 {
		t.Fatalf("lead decides owner's request: %d", code)
	}
	oleg.do("POST", "/api/approvals/access/"+ap.Approvals[0].ExternalID, map[string]any{"approve": true}, nil)
	var p onboarding.Plan
	anna.do("GET", "/api/plan", nil, &p)
	if !stepDone(p, "w1-stash") {
		t.Fatal("stash step must be auto-completed")
	}
	// доступ с автоматической выдачей сразу закрывает шаг
	_, act = anna.ask("добавь меня в канал tech-dep")
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, &conf)
	if conf.Action.Status != domain.ActionApproved {
		t.Fatalf("auto granted: %+v", conf.Action)
	}
	anna.do("GET", "/api/plan", nil, &p)
	if !stepDone(p, "w1-tech-dep") || p.Done != 2 {
		t.Fatalf("plan: done=%d", p.Done)
	}
	// отклонение
	_, act = anna.ask("Запроси доступ к confluence кластера")
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil)
	maria.do("GET", "/api/approvals", nil, &ap)
	if code := maria.do("POST", "/api/approvals/access/"+ap.Approvals[0].ExternalID, map[string]any{"approve": false, "comment": "Сначала команда"}, nil); code != 200 {
		t.Fatal("reject")
	}
	if code := maria.do("POST", "/api/approvals/access/"+ap.Approvals[0].ExternalID, map[string]any{"approve": true}, nil); code != 400 {
		t.Fatal("decide twice")
	}
	if code := maria.do("POST", "/api/approvals/other/x", map[string]any{"approve": true}, nil); code != 400 {
		t.Fatal("bad kind")
	}
	// отмена черновика
	_, act = anna.ask("дай доступ к grafana")
	var c struct{ Action domain.Action }
	if code := anna.do("POST", "/api/actions/"+act.ID+"/cancel", nil, &c); code != 200 || c.Action.Status != domain.ActionCancelled {
		t.Fatal("cancel")
	}
	// чужое действие недоступно
	ivan := e.login("ivan")
	if code := ivan.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil); code != 404 {
		t.Fatal("foreign action")
	}
	// нельзя редактировать служебные поля
	_, act = anna.ask("дай доступ к sentry")
	if code := anna.do("POST", "/api/actions/"+act.ID+"/confirm", map[string]any{"edits": map[string]any{"system": "k8s-prod"}}, nil); code != 400 {
		t.Fatal("edit system must be rejected")
	}
}

func stepDone(p onboarding.Plan, id string) bool {
	for _, per := range p.Periods {
		for _, s := range per.Steps {
			if s.ID == id {
				return s.Status == domain.StepDone
			}
		}
	}
	return false
}

func TestMentorQuestionAndFAQ(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	oleg := e.login("oleg")
	msg, act := anna.ask("Где посмотреть тестовый стенд поиска?")
	if act == nil || act.Kind != domain.ActionMentorQuestion {
		t.Fatalf("escalation: %s", msg.Text)
	}
	var conf struct{ Action domain.Action }
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, &conf)
	if conf.Action.Status != domain.ActionSubmitted || conf.Action.Result["mentor"] != "Олег Кузнецов" {
		t.Fatalf("question sent: %+v", conf.Action)
	}
	var inbox struct{ Questions []domain.Question }
	oleg.do("GET", "/api/inbox", nil, &inbox)
	if len(inbox.Questions) != 1 {
		t.Fatal("inbox")
	}
	if code := anna.do("GET", "/api/inbox", nil, nil); code != 403 {
		t.Fatal("intern inbox")
	}
	maria := e.login("maria")
	if code := maria.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", map[string]any{"answer": "x y"}, nil); code != 403 {
		t.Fatal("foreign mentor answer")
	}
	if code := oleg.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", map[string]any{"answer": ""}, nil); code != 400 {
		t.Fatal("empty answer")
	}
	if code := oleg.do("POST", "/api/inbox/nope/answer", map[string]any{"answer": "ok"}, nil); code != 404 {
		t.Fatal("unknown q")
	}
	ans := map[string]any{"answer": "Тестовый стенд поиска — search-staging, ссылка в README сервиса", "addToFaq": true}
	if code := oleg.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", ans, nil); code != 200 {
		t.Fatal("answer")
	}
	if code := oleg.do("POST", "/api/inbox/"+inbox.Questions[0].ID+"/answer", ans, nil); code != 400 {
		t.Fatal("answer twice")
	}
	var my struct{ Questions []domain.Question }
	anna.do("GET", "/api/questions", nil, &my)
	if my.Questions[0].Status != "answered" {
		t.Fatal("my questions")
	}
	var st chatState
	anna.do("GET", "/api/messages", nil, &st)
	last := st.Messages[len(st.Messages)-1]
	if last.Mode != "mentor" || !strings.Contains(last.Text, "search-staging") {
		t.Fatalf("answer in chat: %+v", last)
	}
	// ответ стал частью базы знаний: следующий стажёр получает его сразу
	m, act2 := anna.ask("где тестовый стенд поиска")
	if act2 != nil || !strings.Contains(m.Text, "search-staging") {
		t.Fatalf("faq reuse: %s", m.Text)
	}
	if err := e.app.LoadFAQ(context.Background()); err != nil {
		t.Fatal(err)
	}
	// команда: наставник видит план своего стажёра
	var team struct{ Interns []app.InternSummary }
	oleg.do("GET", "/api/team", nil, &team)
	if len(team.Interns) != 1 || team.Interns[0].User.ID != "u-anna" {
		t.Fatalf("team: %+v", team)
	}
	if code := anna.do("GET", "/api/team", nil, nil); code != 403 {
		t.Fatal("intern team")
	}
	var ip struct{ Plan onboarding.Plan }
	if code := oleg.do("GET", "/api/team/u-anna/plan", nil, &ip); code != 200 || ip.Plan.Total == 0 {
		t.Fatal("intern plan for mentor")
	}
	if code := oleg.do("GET", "/api/team/u-ivan/plan", nil, nil); code != 403 {
		t.Fatal("foreign intern plan")
	}
	if code := oleg.do("GET", "/api/team/u-maria/plan", nil, nil); code != 404 {
		t.Fatal("non-intern plan")
	}
	elena := e.login("elena")
	elena.do("GET", "/api/team", nil, &team)
	if len(team.Interns) != 1 {
		t.Fatal("hrbp team")
	}
	var in app.Insights
	oleg.do("GET", "/api/insights", nil, &in)
	if in.Escalations != 1 || in.MentorAnswers != 1 || len(in.ByDay) == 0 {
		t.Fatalf("insights: %+v", in)
	}
}

func TestPlanSteps(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	oleg := e.login("oleg")
	var p onboarding.Plan
	if code := anna.do("PATCH", "/api/plan/steps/w1-team", map[string]any{"status": "done"}, &p); code != 200 || p.Done != 1 {
		t.Fatalf("intern step: %d", code)
	}
	if code := anna.do("PATCH", "/api/plan/steps/w1-mentor-1on1", map[string]any{"status": "done"}, nil); code != 400 {
		t.Fatal("mentor step by intern")
	}
	if code := oleg.do("PATCH", "/api/plan/steps/w1-mentor-1on1", map[string]any{"status": "done", "internId": "u-anna"}, &p); code != 200 || p.Done != 2 {
		t.Fatal("mentor step by mentor")
	}
	if code := oleg.do("PATCH", "/api/plan/steps/w1-team", map[string]any{"status": "done", "internId": "u-ivan"}, nil); code != 403 {
		t.Fatal("foreign intern")
	}
	if code := anna.do("PATCH", "/api/plan/steps/w1-team", map[string]any{"status": "weird"}, nil); code != 400 {
		t.Fatal("bad status")
	}
	if code := anna.do("PATCH", "/api/plan/steps/nope", map[string]any{"status": "done"}, nil); code != 404 {
		t.Fatal("unknown step")
	}
	if code := anna.do("PATCH", "/api/plan/steps/w1-team", map[string]any{"status": "done", "internId": "ghost"}, nil); code != 404 {
		t.Fatal("unknown intern")
	}
	if code := anna.do("PATCH", "/api/plan/steps/w1-team", map[string]any{"status": "done", "note": strings.Repeat("я", 501)}, nil); code != 400 {
		t.Fatal("long note")
	}
	if m, _ := anna.ask("что мне делать дальше?"); !strings.Contains(m.Text, "Выполнено 2") {
		t.Fatalf("plan in chat: %s", m.Text)
	}
}

func TestAuthAndValidation(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	hc := &http.Client{}
	get := func(path string) int {
		resp, _ := hc.Get(e.ts.URL + path)
		resp.Body.Close()
		return resp.StatusCode
	}
	if get("/api/health") != 200 || get("/api/config") != 200 || get("/api/me") != 401 || get("/api/nope") != 404 || get("/metrics") != 200 {
		t.Fatal("public endpoints")
	}
	if get("/api/ready") != 200 {
		t.Fatal("ready")
	}
	if get("/") != 404 {
		t.Fatal("no web dir")
	}
	// CSRF: без заголовка изменяющий запрос отклоняется
	resp, _ := hc.Post(e.ts.URL+"/api/auth/login", "application/json", strings.NewReader(`{"login":"anna","password":"buddy2026"}`))
	if resp.StatusCode != 403 {
		t.Fatalf("csrf: %d", resp.StatusCode)
	}
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, e: e, hc: &http.Client{Jar: jar}}
	if code := c.do("POST", "/api/auth/login", map[string]string{"login": "anna", "password": "wrong"}, nil); code != 401 {
		t.Fatal("wrong password")
	}
	if code := c.do("POST", "/api/auth/login", "not-an-object", nil); code != 400 {
		t.Fatal("bad json")
	}
	var du struct{ Users []domain.User }
	c.do("GET", "/api/auth/demo-users", nil, &du)
	if len(du.Users) != 8 {
		t.Fatal("demo users")
	}
	anna := e.login("anna")
	var me struct{ User domain.User }
	anna.do("GET", "/api/me", nil, &me)
	if me.User.Name != "Анна Смирнова" {
		t.Fatal("me")
	}
	if code := anna.do("POST", "/api/messages", map[string]string{"text": "  "}, nil); code != 400 {
		t.Fatal("empty message")
	}
	if code := anna.do("POST", "/api/messages", map[string]string{"text": strings.Repeat("а", 2001)}, nil); code != 400 {
		t.Fatal("long message")
	}
	var docs struct{ Docs []map[string]any }
	anna.do("GET", "/api/kb", nil, &docs)
	for _, d := range docs.Docs {
		if d["id"] == "mentor-guide" {
			t.Fatal("kb list leak")
		}
	}
	if code := anna.do("GET", "/api/kb/mentor-guide", nil, nil); code != 404 {
		t.Fatal("kb get leak")
	}
	if code := anna.do("GET", "/api/kb/process-day-off", nil, nil); code != 200 {
		t.Fatal("kb get")
	}
	anna.do("POST", "/api/auth/logout", nil, nil)
	if code := anna.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatal("logout")
	}
	// поддельная cookie
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "forged"})
	r2, _ := hc.Do(req)
	if r2.StatusCode != 401 {
		t.Fatal("forged cookie")
	}
}

func TestRateLimitAndFlags(t *testing.T) {
	off := allOn()
	off.AbsenceActions, off.MentorEscalation, off.AccessActions = false, false, false
	e := newTestEnv(t, off, "")
	anna := e.login("anna")
	m, act := anna.ask("отгул в пятницу")
	if act != nil || !strings.Contains(m.Text, "отключено") {
		t.Fatalf("flag off: %s", m.Text)
	}
	// лимит сообщений
	limited := false
	for i := 0; i < 40; i++ {
		if code := anna.do("POST", "/api/messages", map[string]string{"text": "привет"}, nil); code == 429 {
			limited = true
			break
		}
	}
	e.app.Wait()
	if !limited {
		t.Fatal("rate limit")
	}
	// действие, созданное до отключения флага, не выполняется
	ctx := context.Background()
	for _, k := range []domain.ActionKind{domain.ActionAbsence, domain.ActionMentorQuestion} {
		a := domain.Action{ID: "act-" + string(k), UserID: "u-anna", Kind: k, Status: domain.ActionProposed, Args: map[string]any{"question": "вопрос про стенд"}}
		_ = e.st.SaveAction(ctx, a)
		if code := anna.do("POST", "/api/actions/"+a.ID+"/confirm", nil, nil); code != 409 {
			t.Fatalf("%s disabled: %d", k, code)
		}
	}
}

func TestCorpOutage(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	maria := e.login("maria")
	_, act := anna.ask("отгул в пятницу")
	e.corp.SetOutage(true)
	if code := anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil); code != 503 {
		t.Fatalf("outage confirm: %d", code)
	}
	var a struct{ Actions []domain.Action }
	anna.do("GET", "/api/actions", nil, &a)
	if a.Actions[0].Status != domain.ActionProposed {
		t.Fatal("draft must survive outage")
	}
	if code := maria.do("GET", "/api/approvals", nil, nil); code != 503 {
		t.Fatal("approvals outage")
	}
	if code := maria.do("POST", "/api/approvals/absence/x", map[string]any{"approve": true}, nil); code != 503 {
		t.Fatal("decide outage")
	}
	if m, _ := anna.ask("дай доступ к stash"); !strings.Contains(m.Text, "не отвечает") {
		t.Fatalf("chat outage: %s", m.Text)
	}
	var ready map[string]string
	(&client{t: t, e: e, hc: &http.Client{}}).do("GET", "/api/ready", nil, &ready)
	if ready["corp"] != "outage" {
		t.Fatalf("ready: %v", ready)
	}
	e.corp.SetOutage(false)
	var conf struct{ Action domain.Action }
	if code := anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, &conf); code != 200 || conf.Action.Status != domain.ActionSubmitted {
		t.Fatal("retry after outage")
	}
	// правила изменились к моменту подтверждения (прошла неделя) — действие не выполнено, причина сохранена
	_, act = anna.ask("отгул 12.11")
	e.clock = e.clock.AddDate(0, 0, 7)
	anna = e.login("anna") // сессия истекла за неделю
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, &conf)
	if conf.Action.Status != domain.ActionFailed || conf.Action.Error == "" {
		t.Fatalf("failed: %+v", conf.Action)
	}
	// неподходящий руководитель (чужая команда) получает отказ сервиса
	sergey := e.login("sergey")
	if code := sergey.do("POST", "/api/approvals/absence/"+conf.Action.ExternalID, map[string]any{"approve": true}, nil); code != 400 && code != 403 {
		t.Fatalf("foreign lead decide: %d", code)
	}
}

func TestStaticSPA(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>buddy</html>"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "sw.js"), []byte("self.x=1"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "manifest.webmanifest"), []byte(`{"name":"AI-buddy"}`), 0o644)
	e := newTestEnv(t, allOn(), dir)
	for path, want := range map[string]string{"/": "buddy", "/plan": "buddy", "/assets/app.js": "console"} {
		resp, _ := http.Get(e.ts.URL + path)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), want) {
			t.Fatalf("%s: %s", path, b)
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Fatal("csp")
		}
	}
	resp, _ := http.Get(e.ts.URL + "/manifest.webmanifest")
	if resp.Header.Get("Content-Type") != "application/manifest+json" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("manifest headers: %v", resp.Header)
	}
	resp, _ = http.Get(e.ts.URL + "/sw.js")
	if resp.Header.Get("Cache-Control") != "no-cache" || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("sw headers: %v", resp.Header)
	}
	resp, _ = http.Get(e.ts.URL + "/assets/app.js")
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatal("assets cache")
	}
	e2 := newTestEnv(t, allOn(), t.TempDir())
	if resp, _ := http.Get(e2.ts.URL + "/x"); resp.StatusCode != 404 {
		t.Fatal("no index")
	}
}

func TestAbsenceTitleInApprovals(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	maria := e.login("maria")
	for _, q := range []string{"отгул 12.11", "отгул с 16 по 17 ноября"} {
		_, act := anna.ask(q)
		anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil)
	}
	var ap struct{ Approvals []app.Approval }
	maria.do("GET", "/api/approvals", nil, &ap)
	titles := ap.Approvals[0].Title + "|" + ap.Approvals[1].Title
	if !strings.Contains(titles, "Отгул на 12.11 (чт), 1 раб. день") || !strings.Contains(titles, "Отгул 16.11 (пн)–17.11 (вт), 2 раб. дня") {
		t.Fatalf("titles: %s", titles)
	}
}

func TestDevopsApproval(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna := e.login("anna")
	pavel := e.login("pavel")
	if code := pavel.do("GET", "/api/approvals", nil, nil); code != 200 {
		t.Fatal("engineer approvals")
	}
	// HR BP без делегирования видит пустой список: решать за руководителя он не может
	var empty struct{ Approvals []app.Approval }
	if code := e.login("elena").do("GET", "/api/approvals", nil, &empty); code != 200 || len(empty.Approvals) != 0 {
		t.Fatal("hrbp approvals")
	}
	_, act := anna.ask("нужен доступ к staging кластеру на редактирование для отладки индексатора поиска")
	if act == nil {
		t.Fatal("staging proposal")
	}
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil)
	var ap struct{ Approvals []app.Approval }
	pavel.do("GET", "/api/approvals", nil, &ap)
	if len(ap.Approvals) != 1 || !strings.Contains(ap.Approvals[0].Title, "Kubernetes: staging, редактирование") {
		t.Fatalf("devops approvals: %+v", ap)
	}
	if code := pavel.do("POST", "/api/approvals/access/"+ap.Approvals[0].ExternalID, map[string]any{"approve": true}, nil); code != 200 {
		t.Fatal("devops decide")
	}
	// инженер не согласует чужие отсутствия
	_, abs := anna.ask("хочу отгул 16 ноября")
	if abs == nil {
		t.Fatal("absence proposal")
	}
	var conf struct{ Action domain.Action }
	anna.do("POST", "/api/actions/"+abs.ID+"/confirm", nil, &conf)
	if code := pavel.do("POST", "/api/approvals/absence/"+conf.Action.ExternalID, map[string]any{"approve": true}, nil); code != 403 {
		t.Fatalf("engineer absence: %d", code)
	}
}

func TestCronReminders(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	srv := New(e.app, nil, nil, "", false)
	srv.CronToken = "tok"
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	run := func(token string) (int, app.ReminderReport) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/cron/reminders", nil)
		req.Header.Set("X-Cron-Token", token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var rep app.ReminderReport
		_ = json.NewDecoder(resp.Body).Decode(&rep)
		return resp.StatusCode, rep
	}
	if code, _ := run("bad"); code != 403 {
		t.Fatal("token")
	}
	anna := e.login("anna")
	// заявка и вопрос, которые «зависнут»
	_, act := anna.ask("отгул в пятницу")
	anna.do("POST", "/api/actions/"+act.ID+"/confirm", nil, nil)
	_, q := anna.ask("Где посмотреть тестовый стенд поиска?")
	anna.do("POST", "/api/actions/"+q.ID+"/confirm", nil, nil)
	code, rep := run("tok")
	if code != 200 || rep.Digests != 2 || rep.StaleRequests != 0 { // Анна и Иван получили сводку
		t.Fatalf("first run: %d %+v", code, rep)
	}
	if _, rep = run("tok"); rep.Digests != 0 {
		t.Fatal("digest once a day")
	}
	// прошло двое суток: напоминаем согласующему и наставнику
	e.clock = e.clock.Add(49 * time.Hour)
	_, rep = run("tok")
	if rep.StaleRequests != 1 || rep.OpenQuestions != 1 || rep.Digests != 2 {
		t.Fatalf("stale: %+v", rep)
	}
	maria := e.login("maria")
	var n struct{ Notifications []domain.Notification }
	maria.do("GET", "/api/notifications", nil, &n)
	if !strings.Contains(n.Notifications[0].Text, "ждёт решения 2 дн.") {
		t.Fatalf("lead reminder: %+v", n.Notifications)
	}
	anna = e.login("anna")
	var st chatState
	anna.do("GET", "/api/messages", nil, &st)
	last := st.Messages[len(st.Messages)-1]
	if last.Mode != "proactive" || !strings.Contains(last.Text, "Доброе утро, Анна") || !strings.Contains(last.Text, "Начинается этап") && !strings.Contains(last.Text, "Следующий шаг") {
		t.Fatalf("digest: %+v", last)
	}
	// флаг выключен
	off := allOn()
	off.Proactive = false
	e2 := newTestEnv(t, off, "")
	if _, err := e2.app.RunReminders(context.Background()); err == nil {
		t.Fatal("flag off")
	}
	e2.app.StartReminders(context.Background(), time.Millisecond) // выключено флагом — ничего не запускает
	ctx, cancel := context.WithCancel(context.Background())
	e.app.StartReminders(ctx, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	cancel()
}

func TestGoals(t *testing.T) {
	e := newTestEnv(t, allOn(), "")
	anna, maria, oleg, ivan := e.login("anna"), e.login("maria"), e.login("oleg"), e.login("ivan")
	if m, _ := anna.ask("какие у меня цели на стажировку?"); !strings.Contains(m.Text, "пока не поставлены") {
		t.Fatalf("no goals: %s", m.Text)
	}
	in := map[string]any{"internId": "u-anna", "title": "Закрыть первую задачу SEARCH-101", "metric": "MR прошёл ревью и выкачен на staging", "due": "2026-11-24"}
	var out struct{ Goal domain.Goal }
	if code := anna.do("POST", "/api/goals", in, nil); code != 403 {
		t.Fatal("intern cannot set goals")
	}
	if code := e.login("sergey").do("POST", "/api/goals", in, nil); code != 403 {
		t.Fatal("foreign lead")
	}
	for k, v := range map[string]any{"title": "x", "metric": "y", "due": "завтра"} {
		bad := map[string]any{}
		for kk, vv := range in {
			bad[kk] = vv
		}
		bad[k] = v
		if code := maria.do("POST", "/api/goals", bad, nil); code != 400 {
			t.Fatalf("validation %s", k)
		}
	}
	if code := maria.do("POST", "/api/goals", map[string]any{"internId": "u-maria"}, nil); code != 404 {
		t.Fatal("goal for non-intern")
	}
	if code := maria.do("POST", "/api/goals", in, &out); code != 201 || out.Goal.Status != domain.GoalAgreed {
		t.Fatalf("add: %d %+v", code, out)
	}
	in["title"] = "Разобраться в архитектуре поиска"
	oleg.do("POST", "/api/goals", in, nil) // наставник тоже может
	var p onboarding.Plan
	anna.do("GET", "/api/plan", nil, &p)
	if !stepDone(p, "w1-goals") {
		t.Fatal("w1-goals must be auto-completed")
	}
	var list struct{ Goals []domain.Goal }
	anna.do("GET", "/api/goals", nil, &list)
	if len(list.Goals) != 2 {
		t.Fatalf("list: %+v", list)
	}
	if code := maria.do("GET", "/api/goals?internId=u-anna", nil, &list); code != 200 || len(list.Goals) != 2 {
		t.Fatal("lead view")
	}
	if code := ivan.do("GET", "/api/goals?internId=u-anna", nil, nil); code != 403 {
		t.Fatal("foreign intern view")
	}
	if code := anna.do("GET", "/api/goals?internId=ghost", nil, nil); code != 404 {
		t.Fatal("unknown intern")
	}
	id := out.Goal.ID
	p50, note := 50, "Код готов, жду ревью"
	if code := anna.do("PATCH", "/api/goals/"+id, map[string]any{"progress": p50, "note": note}, &out); code != 200 || out.Goal.Status != domain.GoalInProgress || out.Goal.Note != note {
		t.Fatalf("progress: %+v", out.Goal)
	}
	for _, bad := range []map[string]any{{"progress": 120}, {"status": "lost"}, {"note": strings.Repeat("я", 501)}} {
		if code := anna.do("PATCH", "/api/goals/"+id, bad, nil); code != 400 {
			t.Fatalf("bad update %v", bad)
		}
	}
	if code := ivan.do("PATCH", "/api/goals/"+id, map[string]any{"status": "done"}, nil); code != 403 {
		t.Fatal("foreign update")
	}
	if code := anna.do("PATCH", "/api/goals/none", map[string]any{"status": "done"}, nil); code != 404 {
		t.Fatal("unknown goal")
	}
	anna.do("PATCH", "/api/goals/"+id, map[string]any{"status": "done"}, &out)
	if out.Goal.Progress != 100 {
		t.Fatal("done = 100%")
	}
	var n struct{ Notifications []domain.Notification }
	maria.do("GET", "/api/notifications", nil, &n)
	if !strings.Contains(n.Notifications[0].Text, "отметил(а) цель выполненной") {
		t.Fatalf("lead notified: %+v", n.Notifications)
	}
	var team struct{ Interns []app.InternSummary }
	maria.do("GET", "/api/team", nil, &team)
	if team.Interns[0].GoalsTotal != 2 || team.Interns[0].GoalsDone != 1 {
		t.Fatalf("team goals: %+v", team.Interns[0])
	}
	if m, _ := anna.ask("какие у меня цели на стажировку?"); !strings.Contains(m.Text, "SEARCH-101") || !strings.Contains(m.Text, "выполнена") {
		t.Fatalf("goals in chat: %s", m.Text)
	}
	// стартовые цели не перезаписывают существующие
	seed := map[string][]app.GoalInput{"u-anna": {{Title: "x", Metric: "y", Due: "2026-12-01"}}, "u-ivan": {{Title: "Цель Ивана", Metric: "критерий", Due: "2026-12-01"}}}
	if err := e.app.SeedGoals(context.Background(), seed, map[string]string{"u-ivan": "u-sergey"}); err != nil {
		t.Fatal(err)
	}
	ivan.do("GET", "/api/goals", nil, &list)
	if len(list.Goals) != 1 {
		t.Fatal("seed ivan")
	}
	ivan.do("GET", "/api/plan", nil, &p)
	if !stepDone(p, "w1-goals") {
		t.Fatal("seed marks w1-goals")
	}
	anna.do("GET", "/api/goals", nil, &list)
	if len(list.Goals) != 2 {
		t.Fatal("seed must not touch anna")
	}
}
