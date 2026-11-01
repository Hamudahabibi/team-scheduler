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
	Log *slog.Logger
	// Loc — часовой пояс киберцентра для разбора дат без смещения.
	Loc          *time.Location
	DB           Pinger
	Auth         *service.Auth
	Teams        *service.Teams
	Rooms        *service.Rooms
	Academic     *service.Academic
	Availability *service.Availability
	Planning     *service.Planning
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

	mux.HandleFunc("GET "+v1+"/disciplines", a.authed(a.listDisciplines))
	mux.HandleFunc("GET "+v1+"/teams", a.authed(a.listTeams))
	mux.HandleFunc("POST "+v1+"/teams", a.authed(a.createTeam))
	mux.HandleFunc("GET "+v1+"/me/teams", a.authed(a.myTeams))
	mux.HandleFunc("GET "+v1+"/teams/{id}", a.authed(a.getTeam))
	mux.HandleFunc("PUT "+v1+"/teams/{id}", a.authed(a.updateTeam))
	mux.HandleFunc("DELETE "+v1+"/teams/{id}", a.authed(a.deleteTeam))
	mux.HandleFunc("PUT "+v1+"/teams/{id}/members/{userID}", a.authed(a.setMember))
	mux.HandleFunc("DELETE "+v1+"/teams/{id}/members/{userID}", a.authed(a.removeMember))
	mux.HandleFunc("GET "+v1+"/teams/{id}/plan", a.authed(a.getPlan))
	mux.HandleFunc("PUT "+v1+"/teams/{id}/plan", a.authed(a.setPlan))

	mux.HandleFunc("GET "+v1+"/rooms", a.authed(a.listRooms))
	mux.HandleFunc("POST "+v1+"/rooms", a.authed(a.createRoom))
	mux.HandleFunc("GET "+v1+"/rooms/{id}", a.authed(a.getRoom))
	mux.HandleFunc("PUT "+v1+"/rooms/{id}", a.authed(a.updateRoom))
	mux.HandleFunc("DELETE "+v1+"/rooms/{id}", a.authed(a.deleteRoom))
	mux.HandleFunc("PUT "+v1+"/rooms/{id}/hours", a.authed(a.setRoomHours))
	mux.HandleFunc("GET "+v1+"/rooms/{id}/blocks", a.authed(a.listRoomBlocks))
	mux.HandleFunc("POST "+v1+"/rooms/{id}/blocks", a.authed(a.addRoomBlock))
	mux.HandleFunc("DELETE "+v1+"/rooms/{id}/blocks/{blockID}", a.authed(a.deleteRoomBlock))

	mux.HandleFunc("GET "+v1+"/groups", a.authed(a.listGroups))
	mux.HandleFunc("POST "+v1+"/groups", a.authed(a.createGroup))
	mux.HandleFunc("GET "+v1+"/groups/{id}/classes", a.authed(a.listClasses))
	mux.HandleFunc("PUT "+v1+"/groups/{id}/classes", a.authed(a.importClasses))

	mux.HandleFunc("GET "+v1+"/me/availability", a.authed(a.myRules))
	mux.HandleFunc("PUT "+v1+"/me/availability", a.authed(a.setMyRules))
	mux.HandleFunc("GET "+v1+"/me/exceptions", a.authed(a.myExceptions))
	mux.HandleFunc("POST "+v1+"/me/exceptions", a.authed(a.addMyException))
	mux.HandleFunc("DELETE "+v1+"/me/exceptions/{id}", a.authed(a.deleteMyException))
	mux.HandleFunc("GET "+v1+"/me/week", a.authed(a.myWeek))
	mux.HandleFunc("GET "+v1+"/teams/{id}/heatmap", a.authed(a.teamHeatmap))

	mux.HandleFunc("POST "+v1+"/schedule/runs", a.authed(a.generateRun))
	mux.HandleFunc("GET "+v1+"/schedule/runs", a.authed(a.listRuns))
	mux.HandleFunc("GET "+v1+"/schedule/runs/{id}", a.authed(a.getRun))
	mux.HandleFunc("POST "+v1+"/schedule/runs/{id}/publish", a.authed(a.publishRun))
	mux.HandleFunc("POST "+v1+"/schedule/runs/{id}/discard", a.authed(a.discardRun))
	mux.HandleFunc("GET "+v1+"/schedule", a.authed(a.weekSchedule))
	mux.HandleFunc("GET "+v1+"/me/schedule", a.authed(a.mySchedule))
	mux.HandleFunc("POST "+v1+"/sessions/{id}/cancel", a.authed(a.cancelSession))
	mux.HandleFunc("GET "+v1+"/teams/{id}/calendar.ics", a.authed(a.teamCalendar))

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
