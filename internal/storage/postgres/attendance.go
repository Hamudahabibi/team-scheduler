package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// UpsertAttendance сохраняет отметки тренировки. Повторная отметка
// игрока перезаписывает предыдущую.
func (s *Store) UpsertAttendance(ctx context.Context, marks []domain.AttendanceMark) error {
	return s.InTx(ctx, func(ctx context.Context) error {
		for _, m := range marks {
			if _, err := s.db(ctx).Exec(ctx, `
				INSERT INTO attendance (session_id, user_id, status, marked_by) VALUES ($1, $2, $3, $4)
				ON CONFLICT (session_id, user_id) DO UPDATE
				SET status = EXCLUDED.status, marked_by = EXCLUDED.marked_by, marked_at = now()`,
				m.SessionID, m.UserID, m.Status, m.MarkedBy); err != nil {
				return mapErr(err, "отметка посещаемости")
			}
		}
		return nil
	})
}

func collectMarks(rows pgx.Rows, err error) ([]domain.AttendanceMark, error) {
	if err != nil {
		return nil, mapErr(err, "посещаемость")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.AttendanceMark, error) {
		var m domain.AttendanceMark
		err := r.Scan(&m.SessionID, &m.UserID, &m.FullName, &m.Status, &m.MarkedBy, &m.MarkedAt)
		return m, err
	})
	return res, mapErr(err, "посещаемость")
}

// AttendanceBySession возвращает отметки тренировки.
func (s *Store) AttendanceBySession(ctx context.Context, sessionID int64) ([]domain.AttendanceMark, error) {
	return collectMarks(s.db(ctx).Query(ctx, `
		SELECT a.session_id, a.user_id, u.full_name, a.status, a.marked_by, a.marked_at
		FROM attendance a JOIN users u ON u.id = a.user_id
		WHERE a.session_id = $1 ORDER BY u.full_name`, sessionID))
}

// TeamAttendance возвращает опубликованные тренировки команды, начавшиеся
// в [from, to), вместе с отметками.
func (s *Store) TeamAttendance(ctx context.Context, teamID int64, from, to time.Time) ([]domain.SessionAttendance, error) {
	rows, err := s.db(ctx).Query(ctx, `
		SELECT id, starts_at, expected_attendance FROM training_sessions
		WHERE team_id = $1 AND status = 'published' AND starts_at >= $2 AND starts_at < $3
		ORDER BY starts_at`, teamID, from, to)
	if err != nil {
		return nil, mapErr(err, "тренировки")
	}
	sessions, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.SessionAttendance, error) {
		var sa domain.SessionAttendance
		err := r.Scan(&sa.SessionID, &sa.StartsAt, &sa.ExpectedAttendance)
		return sa, err
	})
	if err != nil {
		return nil, mapErr(err, "тренировки")
	}

	marks, err := collectMarks(s.db(ctx).Query(ctx, `
		SELECT a.session_id, a.user_id, u.full_name, a.status, a.marked_by, a.marked_at
		FROM attendance a
		JOIN users u ON u.id = a.user_id
		JOIN training_sessions ts ON ts.id = a.session_id
		WHERE ts.team_id = $1 AND ts.status = 'published' AND ts.starts_at >= $2 AND ts.starts_at < $3`,
		teamID, from, to))
	if err != nil {
		return nil, err
	}
	idx := make(map[int64]int, len(sessions))
	for i, sa := range sessions {
		idx[sa.SessionID] = i
	}
	for _, m := range marks {
		if i, ok := idx[m.SessionID]; ok {
			sessions[i].Marks = append(sessions[i].Marks, m)
		}
	}
	return sessions, nil
}
