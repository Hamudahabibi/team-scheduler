package httpapi

import (
	"net/http"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
)

type markRequest struct {
	UserID int64                   `json:"user_id"`
	Status domain.AttendanceStatus `json:"status"`
}

func (a *api) markAttendance(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req []markRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	marks := make([]service.MarkInput, len(req))
	for i, m := range req {
		marks[i] = service.MarkInput{UserID: m.UserID, Status: m.Status}
	}
	saved, err := a.Attendance.Mark(r.Context(), actor, id, marks)
	respond(w, r, http.StatusOK, saved, err)
}

func (a *api) sessionAttendance(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	marks, err := a.Attendance.SessionMarks(r.Context(), actor, id)
	respond(w, r, http.StatusOK, marks, err)
}

func (a *api) teamStats(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
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
	st, err := a.Attendance.TeamStats(r.Context(), actor, id, from, to)
	respond(w, r, http.StatusOK, st, err)
}
