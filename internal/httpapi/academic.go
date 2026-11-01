package httpapi

import (
	"io"
	"mime"
	"net/http"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
)

func (a *api) listGroups(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	groups, err := a.Academic.Groups(r.Context())
	respond(w, r, http.StatusOK, groups, err)
}

type groupRequest struct {
	Code string `json:"code"`
}

func (a *api) createGroup(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var req groupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	g, err := a.Academic.CreateGroup(r.Context(), actor, req.Code)
	respond(w, r, http.StatusCreated, g, err)
}

type importResponse struct {
	Imported int `json:"imported"`
}

// importClasses принимает файл расписания телом запроса. Формат определяется
// по Content-Type: text/calendar или application/json.
func (a *api) importClasses(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
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

	var format service.ScheduleFormat
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch mediaType {
	case "text/calendar":
		format = service.FormatICS
	case "application/json":
		format = service.FormatJSON
	default:
		writeError(w, r, domain.Invalid("Content-Type", "ожидается text/calendar или application/json"))
		return
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, r, domain.Invalid("body", "файл слишком большой или не прочитан"))
		return
	}
	n, err := a.Academic.ImportSchedule(r.Context(), actor, id, format, data, from, to)
	respond(w, r, http.StatusOK, importResponse{Imported: n}, err)
}

func (a *api) listClasses(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
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
	classes, err := a.Academic.Classes(r.Context(), id, from, to)
	respond(w, r, http.StatusOK, classes, err)
}
