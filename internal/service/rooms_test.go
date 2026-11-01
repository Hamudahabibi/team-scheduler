package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

type fakeRoomsRepo struct {
	RoomsRepo
	hours []domain.RoomHours
}

func (f *fakeRoomsRepo) ReplaceRoomHours(_ context.Context, _ int64, h []domain.RoomHours) error {
	f.hours = h
	return nil
}

func (f *fakeRoomsRepo) RoomByID(_ context.Context, id int64) (domain.Room, error) {
	return domain.Room{ID: id, Hours: f.hours}, nil
}

func hours(weekday, open, closeMin int) domain.RoomHours {
	return domain.RoomHours{Weekday: weekday, OpenMin: open, CloseMin: closeMin}
}

func TestRoomSetHours(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRoomsRepo{}
	svc := NewRooms(repo)
	admin := domain.Actor{UserID: 1, Role: domain.RoleAdmin}

	tests := []struct {
		name  string
		hours []domain.RoomHours
		err   error
	}{
		{"будни", []domain.RoomHours{hours(1, 600, 1320), hours(2, 600, 1320)}, nil},
		{"день 8", []domain.RoomHours{hours(8, 600, 1320)}, domain.ErrValidation},
		{"конец раньше начала", []domain.RoomHours{hours(1, 1320, 600)}, domain.ErrValidation},
		{"после полуночи", []domain.RoomHours{hours(1, 600, 1500)}, domain.ErrValidation},
		{"день дважды", []domain.RoomHours{hours(1, 600, 900), hours(1, 1000, 1200)}, domain.ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SetHours(ctx, admin, 1, tt.hours)
			if !errors.Is(err, tt.err) {
				t.Fatalf("ожидалось %v, получено %v", tt.err, err)
			}
		})
	}

	if _, err := svc.SetHours(ctx, domain.Actor{Role: domain.RoleCoach}, 1, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("тренер меняет часы зала: %v", err)
	}
}

func TestRoomValidation(t *testing.T) {
	svc := NewRooms(&fakeRoomsRepo{})
	admin := domain.Actor{UserID: 1, Role: domain.RoleAdmin}
	if _, err := svc.Create(context.Background(), admin, RoomInput{Name: " ", Seats: 10}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("пустое название: %v", err)
	}
	if _, err := svc.Create(context.Background(), admin, RoomInput{Name: "Зал", Seats: 0}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("0 мест: %v", err)
	}
	now := time.Now()
	if _, err := svc.AddBlock(context.Background(), admin, domain.RoomBlock{StartsAt: now, EndsAt: now}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("пустая блокировка: %v", err)
	}
}
