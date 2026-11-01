package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// ListDisciplines возвращает справочник дисциплин.
func (s *Store) ListDisciplines(ctx context.Context) ([]domain.Discipline, error) {
	rows, err := s.q.Query(ctx, `SELECT id, code, name, roster_size FROM disciplines ORDER BY name`)
	if err != nil {
		return nil, mapErr(err, "дисциплины")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Discipline, error) {
		var d domain.Discipline
		err := r.Scan(&d.ID, &d.Code, &d.Name, &d.RosterSize)
		return d, err
	})
	return res, mapErr(err, "дисциплины")
}

// DisciplineByID возвращает дисциплину.
func (s *Store) DisciplineByID(ctx context.Context, id int64) (domain.Discipline, error) {
	var d domain.Discipline
	err := s.q.QueryRow(ctx, `SELECT id, code, name, roster_size FROM disciplines WHERE id = $1`, id).
		Scan(&d.ID, &d.Code, &d.Name, &d.RosterSize)
	return d, mapErr(err, "дисциплина")
}

const teamColumns = `id, name, discipline_id, coach_id, created_at`

func scanTeam(row pgx.Row) (domain.Team, error) {
	var t domain.Team
	err := row.Scan(&t.ID, &t.Name, &t.DisciplineID, &t.CoachID, &t.CreatedAt)
	return t, err
}

// CreateTeam сохраняет команду.
func (s *Store) CreateTeam(ctx context.Context, t domain.Team) (domain.Team, error) {
	created, err := scanTeam(s.q.QueryRow(ctx, `
		INSERT INTO teams (name, discipline_id, coach_id) VALUES ($1, $2, $3)
		RETURNING `+teamColumns, t.Name, t.DisciplineID, t.CoachID))
	return created, mapErr(err, "команда")
}

// UpdateTeam меняет название, дисциплину и тренера команды.
func (s *Store) UpdateTeam(ctx context.Context, t domain.Team) (domain.Team, error) {
	updated, err := scanTeam(s.q.QueryRow(ctx, `
		UPDATE teams SET name = $2, discipline_id = $3, coach_id = $4 WHERE id = $1
		RETURNING `+teamColumns, t.ID, t.Name, t.DisciplineID, t.CoachID))
	return updated, mapErr(err, "команда")
}

// DeleteTeam удаляет команду вместе с составом, планом и тренировками.
func (s *Store) DeleteTeam(ctx context.Context, id int64) error {
	tag, err := s.q.Exec(ctx, `DELETE FROM teams WHERE id = $1`, id)
	return expectOne(tag, err, "команда")
}

// TeamByID возвращает команду без состава.
func (s *Store) TeamByID(ctx context.Context, id int64) (domain.Team, error) {
	t, err := scanTeam(s.q.QueryRow(ctx, `SELECT `+teamColumns+` FROM teams WHERE id = $1`, id))
	return t, mapErr(err, "команда")
}

// ListTeams возвращает все команды.
func (s *Store) ListTeams(ctx context.Context) ([]domain.Team, error) {
	rows, err := s.q.Query(ctx, `SELECT `+teamColumns+` FROM teams ORDER BY name`)
	if err != nil {
		return nil, mapErr(err, "команды")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Team, error) { return scanTeam(r) })
	return res, mapErr(err, "команды")
}

// TeamsByUser возвращает команды, в составе которых есть игрок.
func (s *Store) TeamsByUser(ctx context.Context, userID int64) ([]domain.Team, error) {
	rows, err := s.q.Query(ctx, `
		SELECT t.id, t.name, t.discipline_id, t.coach_id, t.created_at
		FROM teams t JOIN team_members m ON m.team_id = t.id
		WHERE m.user_id = $1 ORDER BY t.name`, userID)
	if err != nil {
		return nil, mapErr(err, "команды")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Team, error) { return scanTeam(r) })
	return res, mapErr(err, "команды")
}

// TeamMembers возвращает состав команды: сначала основной, потом запасные.
func (s *Store) TeamMembers(ctx context.Context, teamID int64) ([]domain.TeamMember, error) {
	rows, err := s.q.Query(ctx, `
		SELECT m.team_id, m.user_id, u.full_name, u.study_group_id, m.role, m.position, m.joined_at
		FROM team_members m JOIN users u ON u.id = m.user_id
		WHERE m.team_id = $1
		ORDER BY m.role = 'substitute', u.full_name`, teamID)
	if err != nil {
		return nil, mapErr(err, "состав")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.TeamMember, error) {
		var m domain.TeamMember
		err := r.Scan(&m.TeamID, &m.UserID, &m.FullName, &m.StudyGroupID, &m.Role, &m.Position, &m.JoinedAt)
		return m, err
	})
	return res, mapErr(err, "состав")
}

// UpsertMember добавляет игрока в состав или меняет его роль и позицию.
func (s *Store) UpsertMember(ctx context.Context, m domain.TeamMember) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role, position) VALUES ($1, $2, $3, $4)
		ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role, position = EXCLUDED.position`,
		m.TeamID, m.UserID, m.Role, m.Position)
	return mapErr(err, "участник команды")
}

// RemoveMember убирает игрока из состава.
func (s *Store) RemoveMember(ctx context.Context, teamID, userID int64) error {
	tag, err := s.q.Exec(ctx, `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID)
	return expectOne(tag, err, "участник команды")
}

const planColumns = `team_id, sessions_per_week, duration_min, min_core_players,
	day_start_min, day_end_min, min_rest_min, priority, updated_at`

func scanPlan(row pgx.Row) (domain.TrainingPlan, error) {
	var p domain.TrainingPlan
	err := row.Scan(&p.TeamID, &p.SessionsPerWeek, &p.DurationMin, &p.MinCorePlayers,
		&p.DayStartMin, &p.DayEndMin, &p.MinRestMin, &p.Priority, &p.UpdatedAt)
	return p, err
}

// UpsertPlan сохраняет тренировочный план команды.
func (s *Store) UpsertPlan(ctx context.Context, p domain.TrainingPlan) (domain.TrainingPlan, error) {
	saved, err := scanPlan(s.q.QueryRow(ctx, `
		INSERT INTO training_plans (team_id, sessions_per_week, duration_min, min_core_players,
			day_start_min, day_end_min, min_rest_min, priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (team_id) DO UPDATE SET
			sessions_per_week = EXCLUDED.sessions_per_week,
			duration_min = EXCLUDED.duration_min,
			min_core_players = EXCLUDED.min_core_players,
			day_start_min = EXCLUDED.day_start_min,
			day_end_min = EXCLUDED.day_end_min,
			min_rest_min = EXCLUDED.min_rest_min,
			priority = EXCLUDED.priority,
			updated_at = now()
		RETURNING `+planColumns,
		p.TeamID, p.SessionsPerWeek, p.DurationMin, p.MinCorePlayers,
		p.DayStartMin, p.DayEndMin, p.MinRestMin, p.Priority))
	return saved, mapErr(err, "тренировочный план")
}

// PlanByTeam возвращает тренировочный план команды.
func (s *Store) PlanByTeam(ctx context.Context, teamID int64) (domain.TrainingPlan, error) {
	p, err := scanPlan(s.q.QueryRow(ctx, `SELECT `+planColumns+` FROM training_plans WHERE team_id = $1`, teamID))
	return p, mapErr(err, "тренировочный план")
}
