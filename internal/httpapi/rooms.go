package httpapi

import (
	"net/http"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
)

type roomRequest struct {
	Name  string `json:"name"`
	Seats int    `json:"seats"`
}

func (a *api) listRooms(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	rooms, err := a.Rooms.List(r.Context())
	respond(w, r, http.StatusOK, rooms, err)
}

func (a *api) getRoom(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	room, err := a.Rooms.Get(r.Context(), id)
	respond(w, r, http.StatusOK, room, err)
}

func (a *api) createRoom(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var req roomRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	room, err := a.Rooms.Create(r.Context(), actor, service.RoomInput{Name: req.Name, Seats: req.Seats})
	respond(w, r, http.StatusCreated, room, err)
}

func (a *api) updateRoom(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req roomRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	room, err := a.Rooms.Update(r.Context(), actor, id, service.RoomInput{Name: req.Name, Seats: req.Seats})
	respond(w, r, http.StatusOK, room, err)
}

func (a *api) deleteRoom(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	respond(w, r, http.StatusNoContent, nil, a.Rooms.Delete(r.Context(), actor, id))
}

func (a *api) setRoomHours(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var hours []domain.RoomHours
	if err := decodeJSON(w, r, &hours); err != nil {
		writeError(w, r, err)
		return
	}
	room, err := a.Rooms.SetHours(r.Context(), actor, id, hours)
	respond(w, r, http.StatusOK, room, err)
}

type roomBlockRequest struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Reason   string    `json:"reason"`
}

func (a *api) addRoomBlock(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req roomBlockRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	b, err := a.Rooms.AddBlock(r.Context(), actor, domain.RoomBlock{
		RoomID: id, StartsAt: req.StartsAt, EndsAt: req.EndsAt, Reason: req.Reason,
	})
	respond(w, r, http.StatusCreated, b, err)
}

func (a *api) listRoomBlocks(w http.ResponseWriter, r *http.Request, _ domain.Actor) {
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
	blocks, err := a.Rooms.Blocks(r.Context(), id, from, to)
	respond(w, r, http.StatusOK, blocks, err)
}

func (a *api) deleteRoomBlock(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	blockID, err := pathID(r, "blockID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	respond(w, r, http.StatusNoContent, nil, a.Rooms.DeleteBlock(r.Context(), actor, id, blockID))
}
