package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// RoomsRepo — хранилище для сервиса залов.
type RoomsRepo interface {
	CreateRoom(ctx context.Context, r domain.Room) (domain.Room, error)
	UpdateRoom(ctx context.Context, r domain.Room) error
	DeleteRoom(ctx context.Context, id int64) error
	ListRooms(ctx context.Context) ([]domain.Room, error)
	RoomByID(ctx context.Context, id int64) (domain.Room, error)
	ReplaceRoomHours(ctx context.Context, roomID int64, hours []domain.RoomHours) error
	CreateRoomBlock(ctx context.Context, b domain.RoomBlock) (domain.RoomBlock, error)
	DeleteRoomBlock(ctx context.Context, roomID, blockID int64) error
	RoomBlocks(ctx context.Context, roomID int64, from, to time.Time) ([]domain.RoomBlock, error)
}

// Rooms — залы киберцентра. Изменения доступны только администратору.
type Rooms struct {
	repo RoomsRepo
}

// NewRooms создаёт сервис залов.
func NewRooms(repo RoomsRepo) *Rooms { return &Rooms{repo: repo} }

// RoomInput — данные зала.
type RoomInput struct {
	Name  string
	Seats int
}

func (in RoomInput) validate() (RoomInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return in, domain.Invalid("name", "укажите название")
	}
	if in.Seats < 1 || in.Seats > 500 {
		return in, domain.Invalid("seats", "от 1 до 500 мест")
	}
	return in, nil
}

// List возвращает залы с часами работы.
func (s *Rooms) List(ctx context.Context) ([]domain.Room, error) {
	return s.repo.ListRooms(ctx)
}

// Get возвращает зал.
func (s *Rooms) Get(ctx context.Context, id int64) (domain.Room, error) {
	return s.repo.RoomByID(ctx, id)
}

// Create добавляет зал.
func (s *Rooms) Create(ctx context.Context, actor domain.Actor, in RoomInput) (domain.Room, error) {
	if !actor.IsAdmin() {
		return domain.Room{}, domain.ErrForbidden
	}
	in, err := in.validate()
	if err != nil {
		return domain.Room{}, err
	}
	r, err := s.repo.CreateRoom(ctx, domain.Room{Name: in.Name, Seats: in.Seats})
	r.Hours = []domain.RoomHours{}
	return r, err
}

// Update меняет зал.
func (s *Rooms) Update(ctx context.Context, actor domain.Actor, id int64, in RoomInput) (domain.Room, error) {
	if !actor.IsAdmin() {
		return domain.Room{}, domain.ErrForbidden
	}
	in, err := in.validate()
	if err != nil {
		return domain.Room{}, err
	}
	if err := s.repo.UpdateRoom(ctx, domain.Room{ID: id, Name: in.Name, Seats: in.Seats}); err != nil {
		return domain.Room{}, err
	}
	return s.repo.RoomByID(ctx, id)
}

// Delete удаляет зал.
func (s *Rooms) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	if !actor.IsAdmin() {
		return domain.ErrForbidden
	}
	return s.repo.DeleteRoom(ctx, id)
}

// SetHours заменяет часы работы зала. Каждый день недели — не больше одного интервала.
func (s *Rooms) SetHours(ctx context.Context, actor domain.Actor, roomID int64, hours []domain.RoomHours) (domain.Room, error) {
	if !actor.IsAdmin() {
		return domain.Room{}, domain.ErrForbidden
	}
	seen := map[int]bool{}
	for i, h := range hours {
		field := fmt.Sprintf("hours[%d]", i)
		if err := domain.ValidateWeekMinutes(field, h.Weekday, h.OpenMin, h.CloseMin); err != nil {
			return domain.Room{}, err
		}
		if seen[h.Weekday] {
			return domain.Room{}, domain.Invalid(field, "день недели указан дважды")
		}
		seen[h.Weekday] = true
	}
	if err := s.repo.ReplaceRoomHours(ctx, roomID, hours); err != nil {
		return domain.Room{}, err
	}
	return s.repo.RoomByID(ctx, roomID)
}

// AddBlock закрывает зал на интервал (мероприятие, ремонт).
func (s *Rooms) AddBlock(ctx context.Context, actor domain.Actor, b domain.RoomBlock) (domain.RoomBlock, error) {
	if !actor.IsAdmin() {
		return domain.RoomBlock{}, domain.ErrForbidden
	}
	if !b.EndsAt.After(b.StartsAt) {
		return domain.RoomBlock{}, domain.Invalid("ends_at", "конец должен быть позже начала")
	}
	b.Reason = strings.TrimSpace(b.Reason)
	return s.repo.CreateRoomBlock(ctx, b)
}

// DeleteBlock снимает блокировку.
func (s *Rooms) DeleteBlock(ctx context.Context, actor domain.Actor, roomID, blockID int64) error {
	if !actor.IsAdmin() {
		return domain.ErrForbidden
	}
	return s.repo.DeleteRoomBlock(ctx, roomID, blockID)
}

// Blocks возвращает блокировки зала за период.
func (s *Rooms) Blocks(ctx context.Context, roomID int64, from, to time.Time) ([]domain.RoomBlock, error) {
	if !to.After(from) {
		return nil, domain.Invalid("to", "конец периода должен быть позже начала")
	}
	return s.repo.RoomBlocks(ctx, roomID, from, to)
}
