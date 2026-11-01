package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
)

// authed оборачивает обработчик, которому нужен вошедший пользователь.
func (a *api) authed(h func(w http.ResponseWriter, r *http.Request, actor domain.Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, r, domain.ErrUnauthorized)
			return
		}
		u, err := a.Auth.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUser, u)
		h(w, r.WithContext(ctx), domain.Actor{UserID: u.ID, Role: u.Role})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(h, "Bearer ")
	if !ok || token == "" {
		return "", false
	}
	return token, true
}

// pathID читает числовой параметр пути, например {id}.
func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.Invalid(name, "ожидается положительное целое число")
	}
	return id, nil
}

type registerRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FullName  string `json:"full_name"`
	GroupCode string `json:"group_code"`
}

func (a *api) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	u, err := a.Auth.Register(r.Context(), service.RegisterInput{
		Email: req.Email, Password: req.Password, FullName: req.FullName, GroupCode: req.GroupCode,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  domain.User `json:"user"`
}

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	token, u, err := a.Auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: u})
}

func (a *api) logout(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	token, _ := bearerToken(r)
	if err := a.Auth.Logout(r.Context(), token); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) me(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	u, err := a.Auth.Me(r.Context(), actor)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

type setGroupRequest struct {
	GroupCode string `json:"group_code"`
}

func (a *api) setMyGroup(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var req setGroupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	u, err := a.Auth.SetMyGroup(r.Context(), actor, req.GroupCode)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *api) listUsers(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	users, err := a.Auth.ListUsers(r.Context(), actor)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type setRoleRequest struct {
	Role domain.Role `json:"role"`
}

func (a *api) setUserRole(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req setRoleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.Auth.SetRole(r.Context(), actor, id, req.Role); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
