// Package api — HTTP API AI-buddy (REST + SSE) и раздача веб-интерфейса.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aibuddy/internal/app"
	"aibuddy/internal/domain"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/kv"
)

func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// CookieName — имя cookie сессии.
const CookieName = "buddy_session"

// Server — HTTP-сервер.
type Server struct {
	// CronToken — токен внутреннего ИИ-крона для POST /api/cron/reminders.
	CronToken string
	app       *app.App
	kb        *knowledge.Base
	log       *slog.Logger
	webDir    string
	secure    bool
}

// New создаёт сервер.
func New(a *app.App, kb *knowledge.Base, log *slog.Logger, webDir string, secureCookie bool) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{app: a, kb: kb, log: log, webDir: webDir, secure: secureCookie}
}

type ctxKey struct{}

func sessionFrom(r *http.Request) kv.Session { return r.Context().Value(ctxKey{}).(kv.Session) }

// Handler собирает маршруты.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/ready", s.ready)
	mux.HandleFunc("GET /api/config", s.config)
	mux.HandleFunc("GET /api/auth/demo-users", s.demoUsers)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/cron/reminders", s.cronReminders)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		s.app.Metrics().WritePrometheus(w)
	})

	auth := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireSession(h)) }
	auth("GET /api/me", s.me)
	auth("GET /api/messages", s.messages)
	auth("POST /api/messages", s.sendMessage)
	auth("POST /api/messages/{id}/feedback", s.feedback)
	auth("GET /api/actions", s.actions)
	auth("POST /api/actions/{id}/confirm", s.confirmAction)
	auth("POST /api/actions/{id}/cancel", s.cancelAction)
	auth("GET /api/plan", s.plan)
	auth("PATCH /api/plan/steps/{step}", s.setStep)
	auth("GET /api/goals", s.goals)
	auth("POST /api/goals", s.addGoal)
	auth("PATCH /api/goals/{id}", s.updateGoal)
	auth("GET /api/team", s.team)
	auth("GET /api/team/{intern}/plan", s.internPlan)
	auth("GET /api/approvals", s.approvals)
	auth("POST /api/approvals/{kind}/{id}", s.decide)
	auth("GET /api/inbox", s.inbox)
	auth("POST /api/inbox/{id}/answer", s.answer)
	auth("GET /api/questions", s.myQuestions)
	auth("GET /api/notifications", s.notifications)
	auth("POST /api/notifications/read", s.readNotifications)
	auth("GET /api/insights", s.insights)
	auth("GET /api/kb", s.kbList)
	auth("GET /api/kb/{id}", s.kbGet)
	auth("GET /api/events", s.events)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeErr(w, 404, "not_found", "Метод не найден")
	})
	mux.HandleFunc("/", s.static)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		// Защита от CSRF: изменяющие запросы обязаны нести собственный заголовок,
		// который браузер не отправит с чужого сайта без CORS.
		if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/cron/") &&
			r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "ai-buddy" {
			writeErr(w, http.StatusForbidden, "csrf", "Нет заголовка X-Requested-With")
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if r.URL.Path != "/api/events" && r.URL.Path != "/api/health" {
			s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) requireSession(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			writeErr(w, 401, "unauthorized", "Нужно войти")
			return
		}
		sess, err := s.app.Session(r.Context(), c.Value)
		if err != nil {
			s.fail(w, err)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, kind, msg string) {
	writeJSON(w, code, map[string]string{"error": kind, "message": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	var inv *app.InvalidError
	switch {
	case errors.As(err, &inv):
		writeErr(w, 400, "invalid", inv.Msg)
	case errors.Is(err, app.ErrUnauthorized):
		writeErr(w, 401, "unauthorized", "Нужно войти")
	case errors.Is(err, app.ErrForbidden):
		writeErr(w, 403, "forbidden", "Недостаточно прав")
	case errors.Is(err, app.ErrNotFound):
		writeErr(w, 404, "not_found", "Не найдено")
	case errors.Is(err, app.ErrDisabled):
		writeErr(w, 409, "disabled", "Функция отключена")
	case errors.Is(err, app.ErrRateLimited):
		writeErr(w, 429, "rate_limited", "Слишком много сообщений — подожди минуту")
	case errors.Is(err, app.ErrUnavailable):
		writeErr(w, 503, "unavailable", "Корпоративный сервис временно недоступен. Попробуй ещё раз.")
	default:
		s.log.Error("internal", "err", err)
		writeErr(w, 500, "internal", "Внутренняя ошибка")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	body := http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "invalid", "Некорректный JSON")
		return false
	}
	return true
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	st := s.app.Ready(r.Context())
	code := 200
	if st["store"] != "ok" || st["kv"] != "ok" {
		code = 503
	}
	writeJSON(w, code, st)
}

// cronReminders — точка вызова для внутреннего ИИ-крона (служебный токен, не cookie).
func (s *Server) cronReminders(w http.ResponseWriter, r *http.Request) {
	if s.CronToken == "" || r.Header.Get("X-Cron-Token") != s.CronToken {
		writeErr(w, 403, "forbidden", "Нужен X-Cron-Token")
		return
	}
	rep, err := s.app.RunReminders(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, rep)
}

func (s *Server) config(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"mode": s.app.Mode(), "flags": s.app.Flags()})
}

func (s *Server) demoUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.app.DemoUsers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"users": users, "password": "buddy2026"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Login, Password string }
	if !decode(w, r, &in) {
		return
	}
	sid, user, err := s.app.Login(r.Context(), in.Login, in.Password)
	if errors.Is(err, app.ErrUnauthorized) {
		writeErr(w, 401, "invalid_credentials", "Неверный логин или пароль")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: sid, Path: "/", HttpOnly: true, Secure: s.secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 12 * 3600})
	writeJSON(w, 200, map[string]any{"user": user})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_ = s.app.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"user": sessionFrom(r).User})
}

func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	m, err := s.app.Messages(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	acts, err := s.app.Actions(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"messages": m, "actions": acts})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	var in struct{ Text string }
	if !decode(w, r, &in) {
		return
	}
	um, am, err := s.app.SendMessage(r.Context(), sessionFrom(r), in.Text)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{"user": um, "assistant": am})
}

func (s *Server) feedback(w http.ResponseWriter, r *http.Request) {
	var in struct{ Helpful bool }
	if !decode(w, r, &in) {
		return
	}
	if err := s.app.Feedback(r.Context(), sessionFrom(r).User, r.PathValue("id"), in.Helpful); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) actions(w http.ResponseWriter, r *http.Request) {
	acts, err := s.app.Actions(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"actions": acts})
}

func (s *Server) confirmAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Edits map[string]any `json:"edits"`
	}
	if !decode(w, r, &in) {
		return
	}
	act, err := s.app.ConfirmAction(r.Context(), sessionFrom(r), r.PathValue("id"), in.Edits)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"action": act})
}

func (s *Server) cancelAction(w http.ResponseWriter, r *http.Request) {
	act, err := s.app.CancelAction(r.Context(), sessionFrom(r).User, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"action": act})
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.PlanFor(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) setStep(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status   domain.StepStatus `json:"status"`
		Note     string            `json:"note"`
		InternID string            `json:"internId"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := sessionFrom(r).User
	if in.InternID == "" {
		in.InternID = u.ID
	}
	p, err := s.app.SetStep(r.Context(), u, in.InternID, r.PathValue("step"), in.Status, in.Note)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) goals(w http.ResponseWriter, r *http.Request) {
	g, err := s.app.Goals(r.Context(), sessionFrom(r).User, r.URL.Query().Get("internId"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"goals": g})
}

func (s *Server) addGoal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InternID string `json:"internId"`
		app.GoalInput
	}
	if !decode(w, r, &in) {
		return
	}
	g, err := s.app.AddGoal(r.Context(), sessionFrom(r).User, in.InternID, in.GoalInput)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"goal": g})
}

func (s *Server) updateGoal(w http.ResponseWriter, r *http.Request) {
	var in app.GoalUpdate
	if !decode(w, r, &in) {
		return
	}
	g, err := s.app.UpdateGoal(r.Context(), sessionFrom(r).User, r.PathValue("id"), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"goal": g})
}

func (s *Server) team(w http.ResponseWriter, r *http.Request) {
	t, err := s.app.Team(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"interns": t})
}

func (s *Server) internPlan(w http.ResponseWriter, r *http.Request) {
	p, intern, err := s.app.InternPlan(r.Context(), sessionFrom(r).User, r.PathValue("intern"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"plan": p, "intern": intern})
}

func (s *Server) approvals(w http.ResponseWriter, r *http.Request) {
	l, err := s.app.Approvals(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"approvals": l})
}

func (s *Server) decide(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Approve bool   `json:"approve"`
		Comment string `json:"comment"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.app.Decide(r.Context(), sessionFrom(r), r.PathValue("kind"), r.PathValue("id"), in.Approve, in.Comment); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	q, err := s.app.Inbox(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"questions": q})
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Answer   string `json:"answer"`
		AddToFAQ bool   `json:"addToFaq"`
	}
	if !decode(w, r, &in) {
		return
	}
	q, err := s.app.AnswerQuestion(r.Context(), sessionFrom(r).User, r.PathValue("id"), in.Answer, in.AddToFAQ)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"question": q})
}

func (s *Server) myQuestions(w http.ResponseWriter, r *http.Request) {
	q, err := s.app.MyQuestions(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"questions": q})
}

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	n, err := s.app.Notifications(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"notifications": n})
}

func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) {
	if err := s.app.MarkNotificationsRead(r.Context(), sessionFrom(r).User); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) insights(w http.ResponseWriter, r *http.Request) {
	in, err := s.app.Insights(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, in)
}

type docView struct {
	ID       string              `json:"id"`
	Title    string              `json:"title"`
	Tags     []string            `json:"tags"`
	Sections []knowledge.Section `json:"sections,omitempty"`
}

func (s *Server) kbList(w http.ResponseWriter, r *http.Request) {
	out := []docView{}
	for _, d := range s.kb.List(sessionFrom(r).User) {
		out = append(out, docView{ID: d.ID, Title: d.Title, Tags: d.Tags})
	}
	writeJSON(w, 200, map[string]any{"docs": out})
}

func (s *Server) kbGet(w http.ResponseWriter, r *http.Request) {
	d, ok := s.kb.Get(r.PathValue("id"), sessionFrom(r).User)
	if !ok {
		s.fail(w, app.ErrNotFound)
		return
	}
	writeJSON(w, 200, docView{ID: d.ID, Title: d.Title, Tags: d.Tags, Sections: d.Sections})
}

// events — поток обновлений (Server-Sent Events).
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "internal", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, cancel := s.app.Hub().Subscribe(sessionFrom(r).User.ID)
	defer cancel()
	fmt.Fprint(w, "retry: 3000\n: connected\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case u, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(u)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

// static раздаёт собранный фронтенд с fallback на index.html (SPA).
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.webDir == "" {
		writeErr(w, 404, "not_found", "Веб-интерфейс не собран")
		return
	}
	clean := filepath.Clean("/" + r.URL.Path)
	p := filepath.Join(s.webDir, clean)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		switch {
		case strings.HasPrefix(clean, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case clean == "/sw.js" || clean == "/manifest.webmanifest":
			// service worker и манифест всегда проверяются заново — иначе обновления не дойдут
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Service-Worker-Allowed", "/")
		}
		http.ServeFile(w, r, p)
		return
	}
	index := filepath.Join(s.webDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		writeErr(w, 404, "not_found", "Веб-интерфейс не собран")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}
