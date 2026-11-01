package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// fakeTeamsRepo реализует только методы, нужные тестам. Вызов остальных
// методов приведёт к панике на nil-интерфейсе, и тест это покажет.
type fakeTeamsRepo struct {
	TeamsRepo
	team    domain.Team
	members []domain.TeamMember
	saved   []domain.TeamMember
	plan    *domain.TrainingPlan
}

func (f *fakeTeamsRepo) TeamByID(_ context.Context, id int64) (domain.Team, error) {
	if id != f.team.ID {
		return domain.Team{}, domain.ErrNotFound
	}
	return f.team, nil
}

func (f *fakeTeamsRepo) DisciplineByID(context.Context, int64) (domain.Discipline, error) {
	return domain.Discipline{ID: 1, Name: "MLBB", RosterSize: 5}, nil
}

func (f *fakeTeamsRepo) TeamMembers(context.Context, int64) ([]domain.TeamMember, error) {
	return f.members, nil
}

func (f *fakeTeamsRepo) UserByID(_ context.Context, id int64) (domain.User, error) {
	return domain.User{ID: id, Role: domain.RolePlayer}, nil
}

func (f *fakeTeamsRepo) UpsertMember(_ context.Context, m domain.TeamMember) error {
	f.saved = append(f.saved, m)
	return nil
}

func (f *fakeTeamsRepo) UpsertPlan(_ context.Context, p domain.TrainingPlan) (domain.TrainingPlan, error) {
	f.plan = &p
	return p, nil
}

func ptr[T any](v T) *T { return &v }

func TestTeamPermissions(t *testing.T) {
	ctx := context.Background()
	repo := &fakeTeamsRepo{team: domain.Team{ID: 1, CoachID: ptr(int64(10))}}
	svc := NewTeams(repo)

	ownCoach := domain.Actor{UserID: 10, Role: domain.RoleCoach}
	otherCoach := domain.Actor{UserID: 11, Role: domain.RoleCoach}
	player := domain.Actor{UserID: 12, Role: domain.RolePlayer}

	in := MemberInput{UserID: 20, Role: domain.MemberCore}
	if err := svc.SetMember(ctx, otherCoach, 1, in); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("чужой тренер: %v", err)
	}
	if err := svc.SetMember(ctx, player, 1, in); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("игрок: %v", err)
	}
	if err := svc.SetMember(ctx, ownCoach, 1, in); err != nil {
		t.Errorf("свой тренер: %v", err)
	}
	if _, err := svc.Create(ctx, ownCoach, TeamInput{Name: "X", DisciplineID: 1}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("тренер создаёт команду: %v", err)
	}
}

func TestSetMemberCoreLimit(t *testing.T) {
	ctx := context.Background()
	repo := &fakeTeamsRepo{team: domain.Team{ID: 1}}
	for i := range 5 {
		repo.members = append(repo.members, domain.TeamMember{UserID: int64(100 + i), Role: domain.MemberCore})
	}
	svc := NewTeams(repo)
	admin := domain.Actor{UserID: 1, Role: domain.RoleAdmin}

	if err := svc.SetMember(ctx, admin, 1, MemberInput{UserID: 200, Role: domain.MemberCore}); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("шестой игрок основного состава: %v", err)
	}
	if err := svc.SetMember(ctx, admin, 1, MemberInput{UserID: 200, Role: domain.MemberSubstitute}); err != nil {
		t.Errorf("запасной: %v", err)
	}
	// Повторное сохранение игрока, который уже в основном составе, не упирается в лимит.
	if err := svc.SetMember(ctx, admin, 1, MemberInput{UserID: 100, Role: domain.MemberCore, Position: "jungle"}); err != nil {
		t.Errorf("смена позиции: %v", err)
	}
	if err := svc.SetMember(ctx, admin, 1, MemberInput{UserID: 201, Role: "captain"}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("неизвестная роль: %v", err)
	}
}

func TestSetPlanChecksRosterSize(t *testing.T) {
	ctx := context.Background()
	repo := &fakeTeamsRepo{team: domain.Team{ID: 1}}
	svc := NewTeams(repo)
	admin := domain.Actor{UserID: 1, Role: domain.RoleAdmin}
	p := domain.TrainingPlan{
		TeamID: 1, SessionsPerWeek: 3, DurationMin: 120, MinCorePlayers: 6,
		DayStartMin: 900, DayEndMin: 1380, Priority: 1,
	}
	if _, err := svc.SetPlan(ctx, admin, p); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("минимум больше состава: %v", err)
	}
	p.MinCorePlayers = 5
	if _, err := svc.SetPlan(ctx, admin, p); err != nil || repo.plan == nil {
		t.Errorf("корректный план: %v", err)
	}
}
