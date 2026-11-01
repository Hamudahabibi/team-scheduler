package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

const runColumns = `id, week_start, algorithm, status, objective, stats, team_ids, created_by, created_at, published_at`

func scanRun(row pgx.Row) (domain.ScheduleRun, error) {
	var r domain.ScheduleRun
	err := row.Scan(&r.ID, &r.WeekStart, &r.Algorithm, &r.Status, &r.Objective, &r.Stats,
		&r.TeamIDs, &r.CreatedBy, &r.CreatedAt, &r.PublishedAt)
	return r, err
}

// sessionSelect выбирает тренировки вместе с названиями команды и зала.
const sessionSelect = `
	SELECT s.id, s.run_id, s.team_id, t.name, s.room_id, r.name, s.starts_at, s.ends_at,
	       s.seats, s.status, s.quality, s.expected_attendance
	FROM training_sessions s
	JOIN teams t ON t.id = s.team_id
	JOIN rooms r ON r.id = s.room_id`

func collectSessions(rows pgx.Rows, err error) ([]domain.TrainingSession, error) {
	if err != nil {
		return nil, mapErr(err, "тренировки")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.TrainingSession, error) {
		var s domain.TrainingSession
		err := r.Scan(&s.ID, &s.RunID, &s.TeamID, &s.TeamName, &s.RoomID, &s.RoomName,
			&s.StartsAt, &s.EndsAt, &s.Seats, &s.Status, &s.Quality, &s.ExpectedAttendance)
		return s, err
	})
	return res, mapErr(err, "тренировки")
}

// CreateRun сохраняет запуск генерации вместе с черновиками тренировок.
func (s *Store) CreateRun(ctx context.Context, run domain.ScheduleRun) (domain.ScheduleRun, error) {
	var created domain.ScheduleRun
	err := s.InTx(ctx, func(ctx context.Context) error {
		var err error
		created, err = scanRun(s.db(ctx).QueryRow(ctx, `
			INSERT INTO schedule_runs (week_start, algorithm, status, objective, stats, team_ids, created_by)
			VALUES ($1, $2, 'draft', $3, $4, $5, $6)
			RETURNING `+runColumns,
			run.WeekStart, run.Algorithm, run.Objective, run.Stats, run.TeamIDs, run.CreatedBy))
		if err != nil {
			return mapErr(err, "запуск генерации")
		}
		_, err = s.db(ctx).CopyFrom(ctx, pgx.Identifier{"training_sessions"},
			[]string{"run_id", "team_id", "room_id", "starts_at", "ends_at", "seats", "status", "quality", "expected_attendance"},
			pgx.CopyFromSlice(len(run.Sessions), func(i int) ([]any, error) {
				ts := run.Sessions[i]
				return []any{created.ID, ts.TeamID, ts.RoomID, ts.StartsAt, ts.EndsAt, ts.Seats,
					string(domain.SessionDraft), ts.Quality, ts.ExpectedAttendance}, nil
			}))
		return mapErr(err, "тренировки")
	})
	if err != nil {
		return domain.ScheduleRun{}, err
	}
	return s.RunByID(ctx, created.ID)
}

// RunByID возвращает запуск с тренировками.
func (s *Store) RunByID(ctx context.Context, id int64) (domain.ScheduleRun, error) {
	run, err := scanRun(s.db(ctx).QueryRow(ctx, `SELECT `+runColumns+` FROM schedule_runs WHERE id = $1`, id))
	if err != nil {
		return run, mapErr(err, "запуск генерации")
	}
	run.Sessions, err = collectSessions(s.db(ctx).Query(ctx,
		sessionSelect+` WHERE s.run_id = $1 ORDER BY s.starts_at, s.team_id`, id))
	return run, err
}

// LockRun блокирует запуск до конца транзакции и возвращает его.
func (s *Store) LockRun(ctx context.Context, id int64) (domain.ScheduleRun, error) {
	run, err := scanRun(s.db(ctx).QueryRow(ctx,
		`SELECT `+runColumns+` FROM schedule_runs WHERE id = $1 FOR UPDATE`, id))
	return run, mapErr(err, "запуск генерации")
}

// RunsByWeek возвращает запуски генерации на неделю, новые первыми.
func (s *Store) RunsByWeek(ctx context.Context, weekStart time.Time) ([]domain.ScheduleRun, error) {
	rows, err := s.db(ctx).Query(ctx,
		`SELECT `+runColumns+` FROM schedule_runs WHERE week_start = $1 ORDER BY id DESC`, weekStart)
	if err != nil {
		return nil, mapErr(err, "запуски генерации")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ScheduleRun, error) { return scanRun(r) })
	return res, mapErr(err, "запуски генерации")
}

// SetRunStatus меняет статус запуска и его черновиков.
func (s *Store) SetRunStatus(ctx context.Context, id int64, status domain.RunStatus, sessionStatus domain.SessionStatus) error {
	return s.InTx(ctx, func(ctx context.Context) error {
		tag, err := s.db(ctx).Exec(ctx, `
			UPDATE schedule_runs SET status = $2,
				published_at = CASE WHEN $2 = 'published' THEN now() ELSE published_at END
			WHERE id = $1`, id, status)
		if err := expectOne(tag, err, "запуск генерации"); err != nil {
			return err
		}
		_, err = s.db(ctx).Exec(ctx,
			`UPDATE training_sessions SET status = $2 WHERE run_id = $1 AND status = 'draft'`, id, sessionStatus)
		return mapErr(err, "тренировки")
	})
}

// LockRooms блокирует строки залов, чтобы параллельные публикации
// в одни и те же залы выполнялись по очереди. Порядок по id исключает deadlock.
func (s *Store) LockRooms(ctx context.Context, roomIDs []int64) ([]domain.Room, error) {
	rows, err := s.db(ctx).Query(ctx,
		`SELECT id, name, seats FROM rooms WHERE id = ANY($1) ORDER BY id FOR UPDATE`, roomIDs)
	if err != nil {
		return nil, mapErr(err, "залы")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Room, error) {
		var room domain.Room
		err := r.Scan(&room.ID, &room.Name, &room.Seats)
		return room, err
	})
	return res, mapErr(err, "залы")
}

// CancelTeamSessions отменяет опубликованные тренировки команд в [from, to).
func (s *Store) CancelTeamSessions(ctx context.Context, teamIDs []int64, from, to time.Time) (int64, error) {
	tag, err := s.db(ctx).Exec(ctx, `
		UPDATE training_sessions SET status = 'cancelled'
		WHERE team_id = ANY($1) AND status = 'published' AND starts_at < $3 AND ends_at > $2`,
		teamIDs, from, to)
	if err != nil {
		return 0, mapErr(err, "тренировки")
	}
	return tag.RowsAffected(), nil
}

// Sessions возвращает тренировки, пересекающие [From, To).
func (s *Store) Sessions(ctx context.Context, f domain.SessionFilter) ([]domain.TrainingSession, error) {
	return collectSessions(s.db(ctx).Query(ctx, sessionSelect+`
		WHERE s.starts_at < $2 AND s.ends_at > $1 AND s.status = $3
		  AND (cardinality($4::bigint[]) = 0 OR s.team_id = ANY($4))
		  AND (cardinality($5::bigint[]) = 0 OR s.room_id = ANY($5))
		ORDER BY s.starts_at, s.team_id`,
		f.From, f.To, f.Status, nonNil(f.TeamIDs), nonNil(f.RoomIDs)))
}

// SessionByID возвращает тренировку.
func (s *Store) SessionByID(ctx context.Context, id int64) (domain.TrainingSession, error) {
	list, err := collectSessions(s.db(ctx).Query(ctx, sessionSelect+` WHERE s.id = $1`, id))
	if err != nil {
		return domain.TrainingSession{}, err
	}
	if len(list) == 0 {
		return domain.TrainingSession{}, mapErr(pgx.ErrNoRows, "тренировка")
	}
	return list[0], nil
}

// SetSessionStatus меняет статус тренировки, если текущий равен from.
func (s *Store) SetSessionStatus(ctx context.Context, id int64, from, to domain.SessionStatus) error {
	tag, err := s.db(ctx).Exec(ctx,
		`UPDATE training_sessions SET status = $3 WHERE id = $1 AND status = $2`, id, from, to)
	return expectOne(tag, err, "тренировка")
}

// nonNil превращает nil в пустой срез: pgx передаёт nil как NULL,
// а cardinality(NULL) — это NULL, а не 0.
func nonNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
