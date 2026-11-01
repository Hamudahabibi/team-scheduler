// Сервер team-scheduler: REST API для планирования тренировок.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/config"
	"github.com/Hamudahabibi/team-scheduler/internal/httpapi"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
	"github.com/Hamudahabibi/team-scheduler/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("сервер остановлен с ошибкой", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool, log); err != nil {
		return err
	}

	store := postgres.NewStore(pool)
	go cleanupTokens(ctx, store, log)

	deps := httpapi.Deps{
		Log:   log,
		Loc:   cfg.Timezone,
		DB:    store,
		Auth:  service.NewAuth(store, cfg.TokenTTL, cfg.AdminEmail),
		Teams: service.NewTeams(store),
		Rooms: service.NewRooms(store),
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(deps),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("HTTP-сервер запущен", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("HTTP-сервер: %w", err)
	case <-ctx.Done():
	}

	log.Info("останавливаюсь")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("остановка HTTP-сервера: %w", err)
	}
	return nil
}

// cleanupTokens раз в час удаляет истёкшие токены доступа.
func cleanupTokens(ctx context.Context, store *postgres.Store, log *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		n, err := store.DeleteExpiredTokens(ctx, time.Now())
		if err != nil && ctx.Err() == nil {
			log.Warn("очистка токенов", slog.Any("err", err))
		} else if n > 0 {
			log.Info("удалены истёкшие токены", slog.Int64("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
