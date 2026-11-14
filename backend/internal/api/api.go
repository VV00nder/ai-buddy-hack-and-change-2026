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
	"aibuddy/internal/onboarding"
	"aibuddy/internal/quiz"
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
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("GET /api/auth/teams", s.teams)
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
	auth("POST /api/goals/{id}/tasks", s.addGoalTask)
	auth("PATCH /api/goals/{id}/tasks/{task}", s.setGoalTask)
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
	auth("POST /api/kb", s.kbSave)
	auth("PUT /api/kb/{id}", s.kbSave)
	auth("DELETE /api/kb/{id}", s.kbDelete)
	auth("POST /api/kb/folders", s.folderCreate)
	auth("DELETE /api/kb/folders", s.folderDelete)
	auth("GET /api/quizzes", s.quizzes)
	auth("GET /api/quizzes/{id}", s.quizTake)
	auth("GET /api/quizzes/{id}/edit", s.quizEdit)
	auth("POST /api/quizzes", s.quizSave)
	auth("PUT /api/quizzes/{id}", s.quizSave)
	auth("DELETE /api/quizzes/{id}", s.quizDelete)
	auth("POST /api/quizzes/{id}/submit", s.quizSubmit)
	auth("POST /api/quizzes/{id}/start", s.quizStart)
	auth("DELETE /api/quizzes/active", s.quizCancel)
	auth("GET /api/quiz-results", s.quizResults)
	auth("GET /api/quiz-results/team", s.teamQuizResults)
	auth("GET /api/templates", s.templates)
	auth("POST /api/templates", s.templateSave)
	auth("PUT /api/templates/{id}", s.templateSave)
	auth("DELETE /api/templates/{id}", s.templateDelete)
	auth("GET /api/team/{intern}/program", s.program)
	auth("PUT /api/team/{intern}/program", s.programSave)
	auth("POST /api/team/{intern}/program/assign", s.programAssign)
	auth("GET /api/department", s.department)
	auth("POST /api/department/{user}/transfer", s.transfer)
	auth("POST /api/department/{user}/assign", s.assignNewcomer)
	auth("GET /api/dashboard", s.dashboard)
	auth("GET /api/team/{intern}/requests", s.internRequests)
	auth("DELETE /api/team/{intern}/requests/{id}", s.deleteRequest)
	auth("DELETE /api/goals/{id}", s.deleteGoal)
	auth("GET /api/requests", s.allRequests)
	auth("DELETE /api/team/{intern}/questions/{id}", s.deleteQuestion)
	auth("PUT /api/team/{intern}/mentors", s.setMentors)
	auth("GET /api/reminders", s.reminders)
	auth("POST /api/reminders", s.addReminder)
	auth("DELETE /api/reminders/{id}", s.deleteReminder)
	auth("GET /api/rights", s.rights)
	auth("POST /api/rights", s.rightSet)
	auth("GET /api/events", s.events)
	auth("GET /api/admin/service", s.adminService)
	auth("PUT /api/admin/flags/{name}", s.adminFlag)
	auth("POST /api/admin/cron", s.adminCron)
	auth("PATCH /api/admin/users/{user}", s.adminSetUser)
	auth("POST /api/admin/users/{user}/password", s.adminPassword)
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
		// новый сотрудник до назначения в отдел видит только профиль и уведомления
		if sess.User.Role == domain.RoleNewcomer && !newcomerAllowed(r.URL.Path) {
			writeErr(w, 403, "awaiting_assignment", "Руководитель ещё не принял вас в отдел")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

func newcomerAllowed(path string) bool {
	return path == "/api/me" || path == "/api/events" || strings.HasPrefix(path, "/api/notifications")
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
	body := http.MaxBytesReader(w, r.Body, 1<<20)
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
	demo := []domain.User{}
	for _, u := range users {
		if !u.Registered { // на экране входа — только тестовые учётные записи
			demo = append(demo, u)
		}
	}
	writeJSON(w, 200, map[string]any{"users": demo, "password": "buddy2026"})
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

func (s *Server) setSession(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: sid, Path: "/", HttpOnly: true, Secure: s.secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 12 * 3600})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in app.RegisterInput
	if !decode(w, r, &in) {
		return
	}
	client := r.Header.Get("X-Forwarded-For")
	if client == "" {
		client, _, _ = strings.Cut(r.RemoteAddr, ":")
	}
	sid, user, err := s.app.Register(r.Context(), in, client)
	if errors.Is(err, app.ErrRateLimited) {
		writeErr(w, 429, "rate_limited", "Слишком много регистраций — попробуйте позже")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.setSession(w, sid)
	writeJSON(w, 201, map[string]any{"user": user})
}

func (s *Server) teams(w http.ResponseWriter, r *http.Request) {
	t, err := s.app.Teams(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"teams": t})
}

func (s *Server) department(w http.ResponseWriter, r *http.Request) {
	d, err := s.app.MyDepartment(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) transfer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Team string `json:"team"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := s.app.Transfer(r.Context(), sessionFrom(r), r.PathValue("user"), in.Team)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) assignNewcomer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role     string `json:"role"`
		MentorID string `json:"mentorId"`
		Team     string `json:"team"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := s.app.AssignNewcomer(r.Context(), sessionFrom(r), r.PathValue("user"), in.Role, in.MentorID, in.Team)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) addGoalTask(w http.ResponseWriter, r *http.Request) {
	var in app.TaskInput
	if !decode(w, r, &in) {
		return
	}
	g, err := s.app.AddGoalTask(r.Context(), sessionFrom(r).User, r.PathValue("id"), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"goal": g})
}

func (s *Server) setGoalTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Done bool `json:"done"`
	}
	if !decode(w, r, &in) {
		return
	}
	g, err := s.app.SetGoalTask(r.Context(), sessionFrom(r).User, r.PathValue("id"), r.PathValue("task"), in.Done)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"goal": g})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.app.Dashboard(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) internRequests(w http.ResponseWriter, r *http.Request) {
	ses, id := sessionFrom(r), r.PathValue("intern")
	rs, err := s.app.InternRequests(r.Context(), ses, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	qs, err := s.app.InternQuestions(r.Context(), ses, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"requests": rs, "questions": qs, "canDelete": s.app.CanManageIntern(r.Context(), ses.User, id),
		"canAnswer": s.app.CanAnswer(r.Context(), ses, id)})
}

func (s *Server) setMentors(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MentorIDs []string `json:"mentorIds"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := s.app.SetMentors(r.Context(), sessionFrom(r), r.PathValue("intern"), in.MentorIDs)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) deleteRequest(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteRequest(r.Context(), sessionFrom(r), r.PathValue("intern"), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deleteQuestion(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteQuestion(r.Context(), sessionFrom(r), r.PathValue("intern"), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) reminders(w http.ResponseWriter, r *http.Request) {
	rs, err := s.app.Reminders(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"reminders": rs})
}

func (s *Server) addReminder(w http.ResponseWriter, r *http.Request) {
	var in app.ReminderInput
	if !decode(w, r, &in) {
		return
	}
	rm, err := s.app.AddReminder(r.Context(), sessionFrom(r).User, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 201, rm)
}

func (s *Server) deleteReminder(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteReminder(r.Context(), sessionFrom(r).User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_ = s.app.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	out := map[string]any{"user": sess.User, "perms": s.app.Perms(r.Context(), sess)}
	if l, ok := s.app.ActiveQuiz(r.Context(), sess.User); ok {
		out["activeQuiz"] = l
	}
	writeJSON(w, 200, out)
}

func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	if s.lockedByQuiz(w, r) {
		return
	}
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
	if s.lockedByQuiz(w, r) {
		return
	}
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
	q, err := s.app.Inbox(r.Context(), sessionFrom(r))
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
	q, err := s.app.AnswerQuestion(r.Context(), sessionFrom(r), r.PathValue("id"), in.Answer, in.AddToFAQ)
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
	ID        string              `json:"id"`
	Title     string              `json:"title"`
	Tags      []string            `json:"tags"`
	Folder    string              `json:"folder"`
	Custom    bool                `json:"custom"`
	Editable  bool                `json:"editable"`
	Audience  []string            `json:"audience,omitempty"`
	Body      string              `json:"body,omitempty"`
	UpdatedBy string              `json:"updatedBy,omitempty"`
	UpdatedAt string              `json:"updatedAt,omitempty"`
	Source    string              `json:"source,omitempty"`
	Sections  []knowledge.Section `json:"sections,omitempty"`
}

func (s *Server) docView(r *http.Request, d *knowledge.Doc, full bool) docView {
	u := sessionFrom(r).User
	v := docView{ID: d.ID, Title: d.Title, Tags: d.Tags, Folder: d.Folder, Custom: d.Custom, Editable: s.app.CanEditDoc(r.Context(), u, d),
		UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt, Source: d.Source}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if full {
		v.Sections = d.Sections
		if v.Editable {
			v.Body, v.Audience = d.Body, d.Audience
		}
	}
	return v
}

func (s *Server) kbList(w http.ResponseWriter, r *http.Request) {
	if s.lockedByQuiz(w, r) {
		return
	}
	u := sessionFrom(r).User
	out := []docView{}
	for _, d := range s.kb.Filter(r.URL.Query().Get("q"), u) {
		out = append(out, s.docView(r, d, false))
	}
	folders, err := s.app.Folders(r.Context(), u)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"docs": out, "folders": folders})
}

func (s *Server) kbGet(w http.ResponseWriter, r *http.Request) {
	if s.lockedByQuiz(w, r) {
		return
	}
	d, ok := s.kb.Get(r.PathValue("id"), sessionFrom(r).User)
	if !ok {
		s.fail(w, app.ErrNotFound)
		return
	}
	writeJSON(w, 200, s.docView(r, d, true))
}

func (s *Server) kbSave(w http.ResponseWriter, r *http.Request) {
	var in app.DocInput
	if !decode(w, r, &in) {
		return
	}
	d, err := s.app.SaveDoc(r.Context(), sessionFrom(r).User, r.PathValue("id"), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	code := 200
	if r.Method == http.MethodPost {
		code = 201
	}
	writeJSON(w, code, s.docView(r, d, true))
}

func (s *Server) kbDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteDoc(r.Context(), sessionFrom(r).User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) folderCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
	}
	if !decode(w, r, &in) {
		return
	}
	f, err := s.app.CreateFolder(r.Context(), sessionFrom(r).User, in.Path)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"folders": f})
}

func (s *Server) folderDelete(w http.ResponseWriter, r *http.Request) {
	f, err := s.app.DeleteFolder(r.Context(), sessionFrom(r).User, r.URL.Query().Get("path"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"folders": f})
}

func (s *Server) quizzes(w http.ResponseWriter, r *http.Request) {
	q, err := s.app.Quizzes(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"quizzes": q})
}

func (s *Server) quizTake(w http.ResponseWriter, r *http.Request) {
	q, err := s.app.QuizToTake(r.Context(), sessionFrom(r).User, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, q)
}

func (s *Server) quizStart(w http.ResponseWriter, r *http.Request) {
	q, err := s.app.StartQuiz(r.Context(), sessionFrom(r).User, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, q)
}

func (s *Server) quizCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.app.CancelQuiz(r.Context(), sessionFrom(r).User); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// lockedByQuiz — во время теста инструкции и чат закрыты (проверка на сервере,
// а не только в интерфейсе).
func (s *Server) lockedByQuiz(w http.ResponseWriter, r *http.Request) bool {
	l, ok := s.app.ActiveQuiz(r.Context(), sessionFrom(r).User)
	if ok {
		writeJSON(w, 423, map[string]string{"error": "quiz_in_progress", "quizId": l.QuizID,
			"message": "Идёт тест «" + l.Title + "» — инструкции и чат откроются после его завершения"})
	}
	return ok
}

func (s *Server) quizEdit(w http.ResponseWriter, r *http.Request) {
	q, err := s.app.QuizToEdit(r.Context(), sessionFrom(r).User, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, q)
}

func (s *Server) quizSave(w http.ResponseWriter, r *http.Request) {
	var in quiz.Quiz
	if !decode(w, r, &in) {
		return
	}
	in.ID = r.PathValue("id")
	q, err := s.app.SaveQuiz(r.Context(), sessionFrom(r).User, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	code := 200
	if r.Method == http.MethodPost {
		code = 201
	}
	writeJSON(w, code, q)
}

func (s *Server) quizDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteQuiz(r.Context(), sessionFrom(r).User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) quizSubmit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Answers []int `json:"answers"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := s.app.SubmitQuiz(r.Context(), sessionFrom(r).User, r.PathValue("id"), in.Answers)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) quizResults(w http.ResponseWriter, r *http.Request) {
	res, err := s.app.QuizResults(r.Context(), sessionFrom(r).User, r.URL.Query().Get("internId"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"attempts": res})
}

func (s *Server) teamQuizResults(w http.ResponseWriter, r *http.Request) {
	res, err := s.app.TeamQuizResults(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"interns": res})
}

func (s *Server) templates(w http.ResponseWriter, r *http.Request) {
	t, err := s.app.Templates(r.Context(), sessionFrom(r).User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"templates": t})
}

func (s *Server) templateSave(w http.ResponseWriter, r *http.Request) {
	var in onboarding.Template
	if !decode(w, r, &in) {
		return
	}
	in.ID = r.PathValue("id")
	t, err := s.app.SaveTemplate(r.Context(), sessionFrom(r).User, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	code := 200
	if r.Method == http.MethodPost {
		code = 201
	}
	writeJSON(w, code, t)
}

func (s *Server) templateDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteTemplate(r.Context(), sessionFrom(r).User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) program(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.ProgramOf(r.Context(), sessionFrom(r).User, r.PathValue("intern"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) programSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Steps []onboarding.Step `json:"steps"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, err := s.app.SaveProgram(r.Context(), sessionFrom(r).User, r.PathValue("intern"), in.Steps)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) programAssign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TemplateID string `json:"templateId"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, err := s.app.AssignTemplate(r.Context(), sessionFrom(r).User, r.PathValue("intern"), in.TemplateID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) rights(w http.ResponseWriter, r *http.Request) {
	m, err := s.app.Rights(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"members": m})
}

func (s *Server) rightSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID  string `json:"userId"`
		Perm    string `json:"perm"`
		Enabled bool   `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	m, err := s.app.SetRight(r.Context(), sessionFrom(r), in.UserID, in.Perm, in.Enabled)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"members": m})
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

// --- Администрирование ---

func (s *Server) adminService(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ServiceInfo(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) adminFlag(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	f, err := s.app.SetFlag(r.Context(), sessionFrom(r), r.PathValue("name"), in.Enabled)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"flags": f})
}

func (s *Server) adminCron(w http.ResponseWriter, r *http.Request) {
	rep, err := s.app.RunCronNow(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, rep)
}

func (s *Server) adminSetUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role     string `json:"role"`
		Team     string `json:"team"`
		MentorID string `json:"mentorId"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := s.app.SetUser(r.Context(), sessionFrom(r), r.PathValue("user"), in.Role, in.Team, in.MentorID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) adminPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.app.ResetPassword(r.Context(), sessionFrom(r), r.PathValue("user"), in.Password); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) allRequests(w http.ResponseWriter, r *http.Request) {
	rs, err := s.app.AllRequests(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"requests": rs})
}

func (s *Server) deleteGoal(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteGoal(r.Context(), sessionFrom(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
