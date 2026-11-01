package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

type fakeAttendanceRepo struct {
	AttendanceRepo
	session  domain.TrainingSession
	team     domain.Team
	members  []domain.TeamMember
	saved    []domain.AttendanceMark
	sessions []domain.SessionAttendance
}

func (f *fakeAttendanceRepo) SessionByID(context.Context, int64) (domain.TrainingSession, error) {
	return f.session, nil
}

func (f *fakeAttendanceRepo) TeamByID(context.Context, int64) (domain.Team, error) {
	return f.team, nil
}

func (f *fakeAttendanceRepo) TeamMembers(context.Context, int64) ([]domain.TeamMember, error) {
	return f.members, nil
}

func (f *fakeAttendanceRepo) UpsertAttendance(_ context.Context, m []domain.AttendanceMark) error {
	f.saved = m
	return nil
}

func (f *fakeAttendanceRepo) AttendanceBySession(context.Context, int64) ([]domain.AttendanceMark, error) {
	return f.saved, nil
}

func (f *fakeAttendanceRepo) TeamAttendance(context.Context, int64, time.Time, time.Time) ([]domain.SessionAttendance, error) {
	return f.sessions, nil
}

func newAttendanceFixture() (*fakeAttendanceRepo, *Attendance) {
	start := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	repo := &fakeAttendanceRepo{
		session: domain.TrainingSession{ID: 1, TeamID: 1, Status: domain.SessionPublished, StartsAt: start},
		team:    domain.Team{ID: 1, CoachID: ptr(int64(10))},
		members: []domain.TeamMember{
			{UserID: 1, FullName: "А", Role: domain.MemberCore},
			{UserID: 2, FullName: "Б", Role: domain.MemberCore},
			{UserID: 3, FullName: "В", Role: domain.MemberSubstitute},
		},
	}
	svc := NewAttendance(repo, 0.5)
	svc.now = func() time.Time { return start.Add(time.Hour) }
	return repo, svc
}

func TestMarkAttendance(t *testing.T) {
	ctx := context.Background()
	repo, svc := newAttendanceFixture()
	coach := domain.Actor{UserID: 10, Role: domain.RoleCoach}

	marks := []MarkInput{{UserID: 1, Status: domain.AttendancePresent}, {UserID: 3, Status: domain.AttendanceLate}}
	if _, err := svc.Mark(ctx, coach, 1, marks); err != nil {
		t.Fatal(err)
	}
	if len(repo.saved) != 2 || *repo.saved[0].MarkedBy != 10 {
		t.Errorf("сохранено: %+v", repo.saved)
	}

	tests := []struct {
		name  string
		actor domain.Actor
		marks []MarkInput
		setup func()
		err   error
	}{
		{"чужой тренер", domain.Actor{UserID: 11, Role: domain.RoleCoach}, marks, nil, domain.ErrForbidden},
		{"игрок не из команды", coach, []MarkInput{{UserID: 99, Status: domain.AttendancePresent}}, nil, domain.ErrValidation},
		{"неизвестный статус", coach, []MarkInput{{UserID: 1, Status: "sleeping"}}, nil, domain.ErrValidation},
		{"тренировка не началась", coach, marks, func() { svc.now = func() time.Time { return repo.session.StartsAt.Add(-time.Minute) } }, domain.ErrConflict},
		{"тренировка отменена", coach, marks, func() { repo.session.Status = domain.SessionCancelled }, domain.ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			if _, err := svc.Mark(ctx, tt.actor, 1, tt.marks); !errors.Is(err, tt.err) {
				t.Fatalf("ожидалось %v, получено %v", tt.err, err)
			}
		})
	}
}

func TestTeamStats(t *testing.T) {
	repo, svc := newAttendanceFixture()
	mark := func(user int64, st domain.AttendanceStatus) domain.AttendanceMark {
		return domain.AttendanceMark{UserID: user, Status: st}
	}
	repo.sessions = []domain.SessionAttendance{
		// Все пришли: 2,5 / 2,5 = 1.
		{SessionID: 1, ExpectedAttendance: 1, Marks: []domain.AttendanceMark{
			mark(1, domain.AttendancePresent), mark(2, domain.AttendanceLate), mark(3, domain.AttendancePresent),
		}},
		// Пришёл один основной: 1 / 2,5 = 0,4.
		{SessionID: 2, ExpectedAttendance: 0.8, Marks: []domain.AttendanceMark{
			mark(1, domain.AttendancePresent), mark(2, domain.AttendanceAbsent), mark(3, domain.AttendanceExcused),
		}},
		// Без отметок — в среднем не участвует.
		{SessionID: 3, ExpectedAttendance: 0.2},
	}
	player := domain.Actor{UserID: 2, Role: domain.RolePlayer}
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	st, err := svc.TeamStats(context.Background(), player, 1, from, from.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if st.Sessions != 3 || st.Marked != 2 {
		t.Errorf("тренировок %d, отмечено %d", st.Sessions, st.Marked)
	}
	if !near(st.AvgExpected, 0.9) || !near(st.AvgActual, 0.7) {
		t.Errorf("прогноз %.3f, факт %.3f", st.AvgExpected, st.AvgActual)
	}
	p2 := st.Players[1]
	if p2.Marked != 2 || p2.Late != 1 || p2.Absent != 1 || !near(p2.Rate, 0.5) {
		t.Errorf("игрок Б: %+v", p2)
	}

	outsider := domain.Actor{UserID: 50, Role: domain.RolePlayer}
	if _, err := svc.TeamStats(context.Background(), outsider, 1, from, from.AddDate(0, 1, 0)); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("посторонний игрок: %v", err)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
