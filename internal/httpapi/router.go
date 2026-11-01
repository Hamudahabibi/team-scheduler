// Package httpapi — HTTP-слой: маршруты, middleware, сериализация.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/service"
)

// Pinger проверяет доступность хранилища для /readyz.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps — зависимости HTTP-слоя.
type Deps struct {
	Log  *slog.Logger
	DB   Pinger
	Auth *service.Auth
}

type api struct {
	Deps
}

// NewRouter собирает все маршруты сервиса.
func NewRouter(d Deps) http.Handler {
	a := &api{Deps: d}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /readyz", a.readyz)

	const v1 = "/api/v1"
	mux.HandleFunc("POST "+v1+"/auth/register", a.register)
	mux.HandleFunc("POST "+v1+"/auth/login", a.login)
	mux.HandleFunc("POST "+v1+"/auth/logout", a.authed(a.logout))
	mux.HandleFunc("GET "+v1+"/me", a.authed(a.me))
	mux.HandleFunc("PUT "+v1+"/me/group", a.authed(a.setMyGroup))
	mux.HandleFunc("GET "+v1+"/users", a.authed(a.listUsers))
	mux.HandleFunc("PUT "+v1+"/users/{id}/role", a.authed(a.setUserRole))

	var h http.Handler = mux
	h = withRecover(h)
	h = withRequestLog(d.Log, h)
	return h
}

func (a *api) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.DB.Ping(ctx); err != nil {
		loggerFrom(r.Context()).Warn("БД недоступна", slog.Any("err", err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
