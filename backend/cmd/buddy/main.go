// buddy — сервис AI-buddy: API, агент, веб-интерфейс.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibuddy/internal/agent"
	"aibuddy/internal/api"
	"aibuddy/internal/app"
	"aibuddy/internal/config"
	"aibuddy/internal/domain"
	"aibuddy/internal/events"
	"aibuddy/internal/flags"
	"aibuddy/internal/knowledge"
	"aibuddy/internal/kv"
	"aibuddy/internal/llm"
	"aibuddy/internal/mcp"
	"aibuddy/internal/onboarding"
	"aibuddy/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.LoadBuddy(nil)
	ff := flags.FromEnv(nil)
	now := func() time.Time { return time.Now().In(cfg.Location) }
	ctx := context.Background()

	var st store.Store = store.NewMemory()
	if cfg.DatabaseURL != "" {
		pg, err := store.OpenPostgres(ctx, cfg.DatabaseURL, 30*time.Second)
		if err != nil {
			return err
		}
		defer pg.Close()
		st = pg
		log.Info("storage: postgres")
	} else {
		log.Warn("storage: in-memory (DATABASE_URL не задан)")
	}
	var sessions kv.KV = kv.NewMemory(now)
	if cfg.RedisURL != "" {
		r, err := kv.OpenRedis(ctx, cfg.RedisURL)
		if err != nil {
			return err
		}
		defer r.Close()
		sessions = r
		log.Info("sessions: redis")
	}
	kb, err := knowledge.Load()
	if err != nil {
		return err
	}
	tpl, err := onboarding.LoadTemplate()
	if err != nil {
		return err
	}
	corpClient := mcp.NewClient(cfg.CorpURL+"/mcp", 10*time.Second)
	var model llm.Client
	if ff.LLM {
		if c, err := llm.New(llm.Config{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Model: cfg.LLMModel,
			Temperature: cfg.LLMTemperature, Timeout: cfg.LLMTimeout}); err == nil {
			model = c
			log.Info("agent: llm", "model", cfg.LLMModel, "base", cfg.LLMBaseURL)
		} else {
			log.Warn("agent: offline (LLM_API_KEY не задан)")
		}
	}
	hub := events.NewHub()
	planFn := func(ctx context.Context, u domain.User) (onboarding.Plan, error) {
		states, err := st.StepStates(ctx, u.ID)
		if err != nil {
			return onboarding.Plan{}, err
		}
		return tpl.Build(u, states, now()), nil
	}
	ag := agent.New(agent.Deps{KB: kb, Corp: corpClient, Plan: planFn, LLM: model, Now: now})
	a := app.New(app.Config{CorpBaseURL: cfg.CorpURL, AgentTimeout: cfg.AgentTimeout, MsgRateLimit: cfg.MsgRateLimit},
		app.Deps{Store: st, KV: sessions, Corp: corpClient, KB: kb, Tpl: tpl, Agent: ag, Hub: hub, Flags: ff, Now: now, Log: log})
	if err := a.LoadFAQ(ctx); err != nil {
		return err
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: api.New(a, kb, log, cfg.WebDir, cfg.CookieSecure).Handler(), ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() {
		log.Info("ai-buddy listening", "addr", cfg.Addr, "mode", ag.Mode())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errc <- err
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errc:
		return err
	case <-stop:
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	a.Wait()
	return nil
}
