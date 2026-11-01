package httpapi

import (
	"net/http"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

type ruleRequest struct {
	Weekday  int             `json:"weekday"`
	StartMin int             `json:"start_min"`
	EndMin   int             `json:"end_min"`
	Kind     domain.RuleKind `json:"kind"`
}

func (a *api) myRules(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	rules, err := a.Availability.Rules(r.Context(), actor)
	respond(w, r, http.StatusOK, rules, err)
}

func (a *api) setMyRules(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var req []ruleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	rules := make([]domain.AvailabilityRule, len(req))
	for i, rr := range req {
		rules[i] = domain.AvailabilityRule{Weekday: rr.Weekday, StartMin: rr.StartMin, EndMin: rr.EndMin, Kind: rr.Kind}
	}
	saved, err := a.Availability.SetRules(r.Context(), actor, rules)
	respond(w, r, http.StatusOK, saved, err)
}

func (a *api) myExceptions(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	from, to, err := a.queryPeriod(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	exc, err := a.Availability.Exceptions(r.Context(), actor, from, to)
	respond(w, r, http.StatusOK, exc, err)
}

type exceptionRequest struct {
	StartsAt time.Time            `json:"starts_at"`
	EndsAt   time.Time            `json:"ends_at"`
	Kind     domain.ExceptionKind `json:"kind"`
	Note     string               `json:"note"`
}

func (a *api) addMyException(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var req exceptionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	e, err := a.Availability.AddException(r.Context(), actor, domain.AvailabilityException{
		StartsAt: req.StartsAt, EndsAt: req.EndsAt, Kind: req.Kind, Note: req.Note,
	})
	respond(w, r, http.StatusCreated, e, err)
}

func (a *api) deleteMyException(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	respond(w, r, http.StatusNoContent, nil, a.Availability.DeleteException(r.Context(), actor, id))
}

func (a *api) myWeek(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	week, err := a.queryTime(r, "week")
	if err != nil {
		writeError(w, r, err)
		return
	}
	view, err := a.Availability.MyWeek(r.Context(), actor, week)
	respond(w, r, http.StatusOK, view, err)
}

func (a *api) teamHeatmap(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	week, err := a.queryTime(r, "week")
	if err != nil {
		writeError(w, r, err)
		return
	}
	h, err := a.Availability.TeamHeatmap(r.Context(), actor, id, week)
	respond(w, r, http.StatusOK, h, err)
}
