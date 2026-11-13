// corpmock — имитация корпоративных сервисов Авито (справочник, отсутствия,
// доступы, SSO), опубликованных как MCP-сервер.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibuddy/internal/config"
	"aibuddy/internal/corp"
	"aibuddy/internal/identity"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.LoadCorp(nil)
	now := func() time.Time { return time.Now().In(cfg.Location) }
	svc, err := corp.New(now, cfg.Location)
	if err != nil {
		log.Error("init", "err", err)
		os.Exit(1)
	}
	if f := os.Getenv("CORP_STATE_FILE"); f != "" {
		if err := svc.UseStateFile(f); err != nil {
			log.Error("state", "err", err)
			os.Exit(1)
		}
		log.Info("corp-mock: регистрации и переводы сохраняются", "file", f)
	}
	signer := identity.NewSigner(cfg.IdentitySecret, now)
	srv := &http.Server{Addr: cfg.Addr, Handler: svc.Handler(signer, cfg.DemoPassword, cfg.AdminToken), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("corp-mock listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
