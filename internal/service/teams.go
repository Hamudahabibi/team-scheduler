package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// TeamsRepo — хранилище для сервиса команд.
type TeamsRepo interface {
	ListDisciplines(ctx context.Context) ([]domain.Discipline, error)
	DisciplineByID(ctx context.Context, id int64) (domain.Discipline, error)
	CreateTeam(ctx context.Context, t domain.Team) (domain.Team, error)
	UpdateTeam(ctx context.Context, t domain.Team) (domain.Team, error)
	DeleteTeam(ctx context.Context, id int64) error
	TeamByID(ctx context.Context, id int64) (domain.Team, error)
	ListTeams(ctx context.Context) ([]domain.Team, error)
	TeamsByUser(ctx context.Context, userID int64) ([]domain.Team, error)
	TeamMembers(ctx context.Context, teamID int64) ([]domain.TeamMember, error)
	UpsertMember(ctx context.Context, m domain.TeamMember) error
	RemoveMember(ctx context.Context, teamID, userID int64) error
	UpsertPlan(ctx context.Context, p domain.TrainingPlan) (domain.TrainingPlan, error)
	PlanByTeam(ctx context.Context, teamID int64) (domain.TrainingPlan, error)
	UserByID(ctx context.Context, id int64) (domain.User, error)
}

// Teams — управление командами, составами и тренировочными планами.
type Teams struct {
	repo TeamsRepo
}

// NewTeams создаёт сервис команд.
func NewTeams(repo TeamsRepo) *Teams { return &Teams{repo: repo} }

// canManageTeam: администратор управляет любой командой, тренер — своей.
func canManageTeam(actor domain.Actor, t domain.Team) bool {
	if actor.IsAdmin() {
		return true
	}
	return actor.Role == domain.RoleCoach && t.CoachID != nil && *t.CoachID == actor.UserID
}

// Disciplines возвращает справочник дисциплин.
func (s *Teams) Disciplines(ctx context.Context) ([]domain.Discipline, error) {
	return s.repo.ListDisciplines(ctx)
}

// TeamInput — данные для создания и изменения команды.
type TeamInput struct {
	Name         string
	DisciplineID int64
	CoachID      *int64
}

func (s *Teams) validateTeamInput(ctx context.Context, in TeamInput) (TeamInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return in, domain.Invalid("name", "укажите название")
	}
	if _, err := s.repo.DisciplineByID(ctx, in.DisciplineID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return in, domain.Invalid("discipline_id", "дисциплина не найдена")
		}
		return in, err
	}
	if in.CoachID != nil {
		coach, err := s.repo.UserByID(ctx, *in.CoachID)
		if errors.Is(err, domain.ErrNotFound) {
			return in, domain.Invalid("coach_id", "пользователь не найден")
		}
		if err != nil {
			return in, err
		}
		if coach.Role == domain.RolePlayer {
			return in, domain.Invalid("coach_id", "пользователю нужно назначить роль coach")
		}
	}
	return in, nil
}

// Create создаёт команду. Только администратор.
func (s *Teams) Create(ctx context.Context, actor domain.Actor, in TeamInput) (domain.Team, error) {
	if !actor.IsAdmin() {
		return domain.Team{}, domain.ErrForbidden
	}
	in, err := s.validateTeamInput(ctx, in)
	if err != nil {
		return domain.Team{}, err
	}
	return s.repo.CreateTeam(ctx, domain.Team{Name: in.Name, DisciplineID: in.DisciplineID, CoachID: in.CoachID})
}

// Update меняет команду. Только администратор: тренер не назначает сам себя.
func (s *Teams) Update(ctx context.Context, actor domain.Actor, id int64, in TeamInput) (domain.Team, error) {
	if !actor.IsAdmin() {
		return domain.Team{}, domain.ErrForbidden
	}
	in, err := s.validateTeamInput(ctx, in)
	if err != nil {
		return domain.Team{}, err
	}
	return s.repo.UpdateTeam(ctx, domain.Team{ID: id, Name: in.Name, DisciplineID: in.DisciplineID, CoachID: in.CoachID})
}

// Delete удаляет команду. Только администратор.
func (s *Teams) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	if !actor.IsAdmin() {
		return domain.ErrForbidden
	}
	return s.repo.DeleteTeam(ctx, id)
}

// List возвращает все команды.
func (s *Teams) List(ctx context.Context) ([]domain.Team, error) {
	return s.repo.ListTeams(ctx)
}

// Mine возвращает команды, в которых состоит пользователь.
func (s *Teams) Mine(ctx context.Context, actor domain.Actor) ([]domain.Team, error) {
	return s.repo.TeamsByUser(ctx, actor.UserID)
}

// Get возвращает команду с составом и планом.
func (s *Teams) Get(ctx context.Context, id int64) (domain.Team, error) {
	t, err := s.repo.TeamByID(ctx, id)
	if err != nil {
		return domain.Team{}, err
	}
	if t.Members, err = s.repo.TeamMembers(ctx, id); err != nil {
		return domain.Team{}, err
	}
	plan, err := s.repo.PlanByTeam(ctx, id)
	switch {
	case err == nil:
		t.Plan = &plan
	case !errors.Is(err, domain.ErrNotFound):
		return domain.Team{}, err
	}
	return t, nil
}

// MemberInput — данные игрока в составе.
type MemberInput struct {
	UserID   int64
	Role     domain.MemberRole
	Position string
}

// SetMember добавляет игрока в состав или меняет его роль.
func (s *Teams) SetMember(ctx context.Context, actor domain.Actor, teamID int64, in MemberInput) error {
	t, err := s.repo.TeamByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !canManageTeam(actor, t) {
		return domain.ErrForbidden
	}
	if !in.Role.Valid() {
		return domain.Invalid("role", "ожидается core или substitute")
	}
	if _, err := s.repo.UserByID(ctx, in.UserID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Invalid("user_id", "пользователь не найден")
		}
		return err
	}
	if in.Role == domain.MemberCore {
		if err := s.checkCoreLimit(ctx, t, in.UserID); err != nil {
			return err
		}
	}
	return s.repo.UpsertMember(ctx, domain.TeamMember{
		TeamID: teamID, UserID: in.UserID, Role: in.Role, Position: strings.TrimSpace(in.Position),
	})
}

// checkCoreLimit не даёт основному составу превысить размер, принятый в дисциплине.
func (s *Teams) checkCoreLimit(ctx context.Context, t domain.Team, userID int64) error {
	d, err := s.repo.DisciplineByID(ctx, t.DisciplineID)
	if err != nil {
		return err
	}
	members, err := s.repo.TeamMembers(ctx, t.ID)
	if err != nil {
		return err
	}
	core := 0
	for _, m := range members {
		if m.Role == domain.MemberCore && m.UserID != userID {
			core++
		}
	}
	if core >= d.RosterSize {
		return fmt.Errorf("в основном составе уже %d игроков (%s): %w", core, d.Name, domain.ErrConflict)
	}
	return nil
}

// RemoveMember убирает игрока из состава. Игрок может выйти из команды сам.
func (s *Teams) RemoveMember(ctx context.Context, actor domain.Actor, teamID, userID int64) error {
	t, err := s.repo.TeamByID(ctx, teamID)
	if err != nil {
		return err
	}
	if !canManageTeam(actor, t) && actor.UserID != userID {
		return domain.ErrForbidden
	}
	return s.repo.RemoveMember(ctx, teamID, userID)
}

// SetPlan сохраняет тренировочный план команды.
func (s *Teams) SetPlan(ctx context.Context, actor domain.Actor, p domain.TrainingPlan) (domain.TrainingPlan, error) {
	t, err := s.repo.TeamByID(ctx, p.TeamID)
	if err != nil {
		return domain.TrainingPlan{}, err
	}
	if !canManageTeam(actor, t) {
		return domain.TrainingPlan{}, domain.ErrForbidden
	}
	if err := p.Validate(); err != nil {
		return domain.TrainingPlan{}, err
	}
	d, err := s.repo.DisciplineByID(ctx, t.DisciplineID)
	if err != nil {
		return domain.TrainingPlan{}, err
	}
	if p.MinCorePlayers > d.RosterSize {
		return domain.TrainingPlan{}, domain.Invalid("min_core_players",
			fmt.Sprintf("больше размера основного состава (%d)", d.RosterSize))
	}
	return s.repo.UpsertPlan(ctx, p)
}

// Plan возвращает тренировочный план команды.
func (s *Teams) Plan(ctx context.Context, teamID int64) (domain.TrainingPlan, error) {
	return s.repo.PlanByTeam(ctx, teamID)
}
