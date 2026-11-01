package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/scheduler"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
)

type generateRequest struct {
	// WeekStart — понедельник недели в формате YYYY-MM-DD.
	WeekStart string              `json:"week_start"`
	Algorithm scheduler.Algorithm `json:"algorithm"`
	TeamIDs   []int64             `json:"team_ids"`
}

func (a *api) generateRun(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var req generateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	week, err := time.ParseInLocation(time.DateOnly, req.WeekStart, a.Loc)
	if err != nil {
		writeError(w, r, domain.Invalid("week_start", "ожидается дата YYYY-MM-DD"))
		return
	}
	run, err := a.Planning.Generate(r.Context(), actor, service.GenerateInput{
		WeekStart: week, Algorithm: req.Algorithm, TeamIDs: req.TeamIDs,
	})
	respond(w, r, http.StatusCreated, run, err)
}

func (a *api) listRuns(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	week, err := a.queryTime(r, "week")
	if err != nil {
		writeError(w, r, err)
		return
	}
	runs, err := a.Planning.Runs(r.Context(), week)
	respond(w, r, http.StatusOK, runs, err)
}

func (a *api) getRun(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	run, err := a.Planning.Run(r.Context(), id)
	respond(w, r, http.StatusOK, run, err)
}

func (a *api) publishRun(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	run, err := a.Planning.Publish(r.Context(), actor, id)
	respond(w, r, http.StatusOK, run, err)
}

func (a *api) discardRun(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	respond(w, r, http.StatusNoContent, nil, a.Planning.Discard(r.Context(), actor, id))
}

func (a *api) weekSchedule(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	week, err := a.queryTime(r, "week")
	if err != nil {
		writeError(w, r, err)
		return
	}
	teamID, err := queryInt64(r, "team_id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	sessions, err := a.Planning.WeekSchedule(r.Context(), week, teamID)
	respond(w, r, http.StatusOK, sessions, err)
}

func (a *api) mySchedule(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	week, err := a.queryTime(r, "week")
	if err != nil {
		writeError(w, r, err)
		return
	}
	s, err := a.Planning.MyWeek(r.Context(), actor, week)
	respond(w, r, http.StatusOK, s, err)
}

func (a *api) cancelSession(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	respond(w, r, http.StatusNoContent, nil, a.Planning.CancelSession(r.Context(), actor, id))
}

func (a *api) teamCalendar(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	from, to, err := a.queryPeriod(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	data, err := a.Planning.TeamCalendar(r.Context(), id, from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="team-`+strconv.FormatInt(id, 10)+`.ics"`)
	_, _ = w.Write(data)
}
