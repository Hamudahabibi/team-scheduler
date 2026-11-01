package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

type fakeAcademicRepo struct {
	AcademicRepo
	replaced []domain.ClassEvent
}

func (f *fakeAcademicRepo) GroupByID(_ context.Context, id int64) (domain.StudyGroup, error) {
	if id != 1 {
		return domain.StudyGroup{}, domain.ErrNotFound
	}
	return domain.StudyGroup{ID: 1, Code: "ИДБ-22-04"}, nil
}

func (f *fakeAcademicRepo) ReplaceClasses(_ context.Context, _ int64, _, _ time.Time, c []domain.ClassEvent) error {
	f.replaced = c
	return nil
}

const weeklyICS = "BEGIN:VCALENDAR\n" +
	"BEGIN:VEVENT\n" +
	"SUMMARY:Базы данных\n" +
	"LOCATION:0310\n" +
	"DTSTART:20261102T083000\n" +
	"DTEND:20261102T101000\n" +
	"RRULE:FREQ=WEEKLY\n" +
	"END:VEVENT\n" +
	"END:VCALENDAR\n"

func TestImportSchedule(t *testing.T) {
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Moscow")
	repo := &fakeAcademicRepo{}
	svc := NewAcademic(repo, loc)
	admin := domain.Actor{UserID: 1, Role: domain.RoleAdmin}
	from := time.Date(2026, 11, 2, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 28)

	n, err := svc.ImportSchedule(ctx, admin, 1, FormatICS, []byte(weeklyICS), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || len(repo.replaced) != 4 {
		t.Fatalf("загружено %d (%d), ожидалось 4 пары за 4 недели", n, len(repo.replaced))
	}
	if repo.replaced[0].Title != "Базы данных (0310)" {
		t.Errorf("название: %q", repo.replaced[0].Title)
	}

	json := `[{"title":"Физра","starts_at":"2026-11-03T10:20:00+03:00","ends_at":"2026-11-03T12:00:00+03:00"},
	          {"title":"Вне периода","starts_at":"2027-01-03T10:20:00+03:00","ends_at":"2027-01-03T12:00:00+03:00"}]`
	n, err = svc.ImportSchedule(ctx, admin, 1, FormatJSON, []byte(json), from, to)
	if err != nil || n != 1 {
		t.Fatalf("JSON: %d, %v", n, err)
	}

	errCases := []struct {
		name   string
		actor  domain.Actor
		group  int64
		format ScheduleFormat
		data   string
		to     time.Time
		err    error
	}{
		{"не админ", domain.Actor{Role: domain.RoleCoach}, 1, FormatICS, weeklyICS, to, domain.ErrForbidden},
		{"нет группы", admin, 2, FormatICS, weeklyICS, to, domain.ErrNotFound},
		{"пустой период", admin, 1, FormatICS, weeklyICS, from, domain.ErrValidation},
		{"больше года", admin, 1, FormatICS, weeklyICS, from.AddDate(2, 0, 0), domain.ErrValidation},
		{"битый iCalendar", admin, 1, FormatICS, "BEGIN:VEVENT\nDTSTART:x\nEND:VEVENT", to, domain.ErrValidation},
		{"битый JSON", admin, 1, FormatJSON, "{", to, domain.ErrValidation},
		{"неизвестный формат", admin, 1, "xlsx", "", to, domain.ErrValidation},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ImportSchedule(ctx, tc.actor, tc.group, tc.format, []byte(tc.data), from, tc.to)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ожидалось %v, получено %v", tc.err, err)
			}
		})
	}
}
