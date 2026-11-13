// Package app — сервисный слой AI-buddy: сессии, диалог, действия,
// план адаптации, сценарии наставника и руководителя, метрики.
package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"aibuddy/internal/agent"
	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/flags"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/kv"
	"aibuddy/internal/onboarding"
	"aibuddy/internal/quiz"
	"aibuddy/internal/store"
)

// Ошибки уровня сервиса (маппятся в HTTP-коды).
var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrInvalid      = errors.New("invalid input")
	ErrRateLimited  = errors.New("rate limited")
	ErrUnavailable  = errors.New("service unavailable")
	ErrDisabled     = errors.New("feature disabled")
)

// InvalidError — ошибка валидации с текстом для пользователя.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }
func (e *InvalidError) Unwrap() error { return ErrInvalid }

func invalid(format string, a ...any) error { return &InvalidError{Msg: fmt.Sprintf(format, a...)} }

// Config — параметры сервиса.
type Config struct {
	CorpBaseURL   string
	SessionTTL    time.Duration
	MsgRateLimit  int           // сообщений в минуту на пользователя
	AgentTimeout  time.Duration // таймаут обработки сообщения агентом
	MaxMessageLen int
}

// App — сервисный слой.
type App struct {
	cfg   Config
	store store.Store
	kv    kv.KV
	corp  agent.Corp
	kb    *knowledge.Base
	tpl   *onboarding.Template
	// builtinTpl и builtinQuiz — встроенные шаблоны обучения и тесты (только чтение).
	builtinTpl  []*onboarding.Template
	builtinQuiz []*quiz.Quiz
	agent       *agent.Agent
	hub         *events.Hub
	flags       flags.Flags
	now         func() time.Time
	log         *slog.Logger
	http        *http.Client

	cronMu   sync.Mutex
	cronSent map[string]string

	stampMu   sync.Mutex
	lastStamp time.Time

	usersMu sync.Mutex
	users   []domain.User
	wg      sync.WaitGroup
	metrics *Metrics
}

// Deps — зависимости.
type Deps struct {
	Store store.Store
	KV    kv.KV
	Corp  agent.Corp
	KB    *knowledge.Base
	Tpl   *onboarding.Template
	// Templates и Quizzes — встроенные шаблоны и тесты; nil — загрузить встроенные.
	Templates []*onboarding.Template
	Quizzes   []*quiz.Quiz
	Agent     *agent.Agent
	Hub       *events.Hub
	Flags     flags.Flags
	Now       func() time.Time
	Log       *slog.Logger
}

// New создаёт сервис.
func New(cfg Config, d Deps) *App {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.MsgRateLimit == 0 {
		cfg.MsgRateLimit = 20
	}
	if cfg.AgentTimeout == 0 {
		cfg.AgentTimeout = 60 * time.Second
	}
	if cfg.MaxMessageLen == 0 {
		cfg.MaxMessageLen = 2000
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Templates == nil {
		d.Templates, _ = onboarding.LoadTemplates()
	}
	if d.Tpl == nil && len(d.Templates) > 0 {
		d.Tpl = d.Templates[0]
	}
	if d.Quizzes == nil {
		d.Quizzes, _ = quiz.LoadBuiltin()
	}
	return &App{cfg: cfg, store: d.Store, kv: d.KV, corp: d.Corp, kb: d.KB, tpl: d.Tpl, builtinTpl: d.Templates, builtinQuiz: d.Quizzes, agent: d.Agent, hub: d.Hub,
		flags: d.Flags, now: d.Now, log: d.Log, http: &http.Client{Timeout: 10 * time.Second}, metrics: NewMetrics(), cronSent: map[string]string{}}
}

// Hub возвращает хаб обновлений.
func (a *App) Hub() *events.Hub { return a.hub }

// Flags возвращает флаги.
func (a *App) Flags() flags.Flags { return a.flags }

// Mode — режим агента (llm | offline).
func (a *App) Mode() string { return a.agent.Mode() }

// Metrics возвращает счётчики.
func (a *App) Metrics() *Metrics { return a.metrics }

// Wait дожидается фоновых задач (для тестов и корректной остановки).
func (a *App) Wait() { a.wg.Wait() }

// stamp — монотонное время для упорядочивания сообщений.
func (a *App) stamp() time.Time {
	a.stampMu.Lock()
	defer a.stampMu.Unlock()
	t := a.now()
	if !t.After(a.lastStamp) {
		t = a.lastStamp.Add(time.Microsecond)
	}
	a.lastStamp = t
	return t
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// Ready проверяет зависимости.
func (a *App) Ready(ctx context.Context) map[string]string {
	out := map[string]string{"store": "ok", "kv": "ok", "corp": "ok", "llm": a.agent.LLMStatus()}
	if err := a.store.Ping(ctx); err != nil {
		out["store"] = err.Error()
	}
	if err := a.kv.Ping(ctx); err != nil {
		out["kv"] = err.Error()
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.CorpBaseURL+"/healthz", nil)
	if resp, err := a.http.Do(req); err != nil {
		out["corp"] = "unreachable"
	} else {
		var h struct{ Outage bool }
		_ = json.NewDecoder(resp.Body).Decode(&h)
		resp.Body.Close()
		if h.Outage {
			out["corp"] = "outage"
		}
	}
	return out
}

// --- Сессии (авторизация — через имитацию корпоративного SSO) ---

// Login проверяет учётные данные в SSO и создаёт сессию.
func (a *App) Login(ctx context.Context, login, password string) (string, domain.User, error) {
	body, _ := json.Marshal(map[string]string{"login": strings.TrimSpace(strings.ToLower(login)), "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.CorpBaseURL+"/sso/token", bytes.NewReader(body))
	if err != nil {
		return "", domain.User{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", domain.User{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", domain.User{}, ErrUnauthorized
	}
	var out struct {
		Token string      `json:"token"`
		User  domain.User `json:"user"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil || out.Token == "" {
		return "", domain.User{}, ErrUnavailable
	}
	sid := newID("s") + newID("")
	if err := a.kv.SetSession(ctx, sid, kv.Session{User: out.User, Token: out.Token}, a.cfg.SessionTTL); err != nil {
		return "", domain.User{}, err
	}
	a.Track(ctx, out.User.ID, "login", map[string]any{"role": out.User.Role})
	return sid, out.User, nil
}

// Session возвращает сессию.
func (a *App) Session(ctx context.Context, sid string) (kv.Session, error) {
	if sid == "" {
		return kv.Session{}, ErrUnauthorized
	}
	s, err := a.kv.GetSession(ctx, sid)
	if errors.Is(err, kv.ErrNoSession) {
		return s, ErrUnauthorized
	}
	return s, err
}

// Logout завершает сессию.
func (a *App) Logout(ctx context.Context, sid string) error { return a.kv.DeleteSession(ctx, sid) }

// DemoUsers — тестовые учётные записи для экрана входа.
func (a *App) DemoUsers(ctx context.Context) ([]domain.User, error) {
	a.usersMu.Lock()
	defer a.usersMu.Unlock()
	if a.users != nil {
		return a.users, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.CorpBaseURL+"/sso/users", nil)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	var out struct {
		Users []domain.User `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Users) == 0 {
		return nil, ErrUnavailable
	}
	a.users = out.Users
	return a.users, nil
}

func (a *App) userByID(ctx context.Context, id string) (domain.User, bool) {
	users, err := a.DemoUsers(ctx)
	if err != nil {
		return domain.User{}, false
	}
	for _, u := range users {
		if u.ID == id {
			return u, true
		}
	}
	return domain.User{}, false
}

// --- Уведомления и события ---

func (a *App) notify(ctx context.Context, userID, kind, text, link string) {
	n := domain.Notification{ID: newID("n"), UserID: userID, Kind: kind, Text: text, Link: link, CreatedAt: a.now()}
	if err := a.store.AddNotification(ctx, n); err != nil {
		a.log.Error("notify", "err", err)
		return
	}
	a.hub.Publish(userID, events.Update{Type: "notification", Data: n})
}

// Notifications — уведомления пользователя.
func (a *App) Notifications(ctx context.Context, u domain.User) ([]domain.Notification, error) {
	return a.store.Notifications(ctx, u.ID, 30)
}

// MarkNotificationsRead помечает уведомления прочитанными.
func (a *App) MarkNotificationsRead(ctx context.Context, u domain.User) error {
	return a.store.MarkNotificationsRead(ctx, u.ID)
}

// Track записывает продуктовое событие (форма совместима с clickstream).
func (a *App) Track(ctx context.Context, userID, typ string, props map[string]any) {
	e := domain.Event{ID: newID("e"), UserID: userID, Type: typ, Props: props, At: a.now()}
	a.metrics.Inc(typ)
	if err := a.store.AddEvent(ctx, e); err != nil {
		a.log.Error("track", "err", err)
	}
	a.log.Info("event", "type", typ, "user", userID, "props", props)
}
