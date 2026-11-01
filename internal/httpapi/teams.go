package httpapi

import (
	"net/http"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
)

func (a *api) listDisciplines(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	ds, err := a.Teams.Disciplines(r.Context())
	respond(w, r, http.StatusOK, ds, err)
}

func (a *api) listTeams(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	teams, err := a.Teams.List(r.Context())
	respond(w, r, http.StatusOK, teams, err)
}

func (a *api) myTeams(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	teams, err := a.Teams.Mine(r.Context(), actor)
	respond(w, r, http.StatusOK, teams, err)
}

type teamRequest struct {
	Name         string `json:"name"`
	DisciplineID int64  `json:"discipline_id"`
	CoachID      *int64 `json:"coach_id"`
}

func (req teamRequest) input() service.TeamInput {
	return service.TeamInput{Name: req.Name, DisciplineID: req.DisciplineID, CoachID: req.CoachID}
}

func (a *api) createTeam(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var req teamRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	t, err := a.Teams.Create(r.Context(), actor, req.input())
	respond(w, r, http.StatusCreated, t, err)
}

func (a *api) getTeam(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	t, err := a.Teams.Get(r.Context(), id)
	respond(w, r, http.StatusOK, t, err)
}

func (a *api) updateTeam(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req teamRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	t, err := a.Teams.Update(r.Context(), actor, id, req.input())
	respond(w, r, http.StatusOK, t, err)
}

func (a *api) deleteTeam(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	respond(w, r, http.StatusNoContent, nil, a.Teams.Delete(r.Context(), actor, id))
}

type memberRequest struct {
	Role     domain.MemberRole `json:"role"`
	Position string            `json:"position"`
}

func (a *api) setMember(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	teamID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	userID, err := pathID(r, "userID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req memberRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	err = a.Teams.SetMember(r.Context(), actor, teamID, service.MemberInput{
		UserID: userID, Role: req.Role, Position: req.Position,
	})
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *api) removeMember(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	teamID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	userID, err := pathID(r, "userID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	respond(w, r, http.StatusNoContent, nil, a.Teams.RemoveMember(r.Context(), actor, teamID, userID))
}

type planRequest struct {
	SessionsPerWeek int `json:"sessions_per_week"`
	DurationMin     int `json:"duration_min"`
	MinCorePlayers  int `json:"min_core_players"`
	DayStartMin     int `json:"day_start_min"`
	DayEndMin       int `json:"day_end_min"`
	MinRestMin      int `json:"min_rest_min"`
	Priority        int `json:"priority"`
}

func (a *api) getPlan(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Teams.Plan(r.Context(), id)
	respond(w, r, http.StatusOK, p, err)
}

func (a *api) setPlan(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	req := planRequest{Priority: 1}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.Teams.SetPlan(r.Context(), actor, domain.TrainingPlan{
		TeamID:          id,
		SessionsPerWeek: req.SessionsPerWeek,
		DurationMin:     req.DurationMin,
		MinCorePlayers:  req.MinCorePlayers,
		DayStartMin:     req.DayStartMin,
		DayEndMin:       req.DayEndMin,
		MinRestMin:      req.MinRestMin,
		Priority:        req.Priority,
	})
	respond(w, r, http.StatusOK, p, err)
}
