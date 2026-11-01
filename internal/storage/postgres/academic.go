package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// ListGroups возвращает учебные группы по алфавиту.
func (s *Store) ListGroups(ctx context.Context) ([]domain.StudyGroup, error) {
	rows, err := s.q.Query(ctx, `SELECT id, code FROM study_groups ORDER BY code`)
	if err != nil {
		return nil, mapErr(err, "учебные группы")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.StudyGroup, error) {
		var g domain.StudyGroup
		err := r.Scan(&g.ID, &g.Code)
		return g, err
	})
	return res, mapErr(err, "учебные группы")
}

// CreateGroup создаёт учебную группу. Повтор кода — конфликт.
func (s *Store) CreateGroup(ctx context.Context, code string) (domain.StudyGroup, error) {
	g := domain.StudyGroup{Code: code}
	err := s.q.QueryRow(ctx, `INSERT INTO study_groups (code) VALUES ($1) RETURNING id`, code).Scan(&g.ID)
	return g, mapErr(err, "учебная группа")
}

// GroupByID возвращает учебную группу.
func (s *Store) GroupByID(ctx context.Context, id int64) (domain.StudyGroup, error) {
	var g domain.StudyGroup
	err := s.q.QueryRow(ctx, `SELECT id, code FROM study_groups WHERE id = $1`, id).Scan(&g.ID, &g.Code)
	return g, mapErr(err, "учебная группа")
}

// ReplaceClasses атомарно заменяет занятия группы, начинающиеся в [from, to).
// Повторный импорт того же периода не создаёт дублей.
func (s *Store) ReplaceClasses(ctx context.Context, groupID int64, from, to time.Time, classes []domain.ClassEvent) error {
	return s.InTx(ctx, func(tx *Store) error {
		var id int64
		if err := tx.q.QueryRow(ctx, `SELECT id FROM study_groups WHERE id = $1 FOR UPDATE`, groupID).Scan(&id); err != nil {
			return mapErr(err, "учебная группа")
		}
		if _, err := tx.q.Exec(ctx,
			`DELETE FROM class_events WHERE group_id = $1 AND starts_at >= $2 AND starts_at < $3`,
			groupID, from, to); err != nil {
			return mapErr(err, "занятия")
		}
		_, err := tx.q.CopyFrom(ctx, pgx.Identifier{"class_events"},
			[]string{"group_id", "title", "starts_at", "ends_at"},
			pgx.CopyFromSlice(len(classes), func(i int) ([]any, error) {
				c := classes[i]
				return []any{groupID, c.Title, c.StartsAt, c.EndsAt}, nil
			}))
		return mapErr(err, "занятия")
	})
}

// ClassesByGroups возвращает занятия групп, пересекающие [from, to).
func (s *Store) ClassesByGroups(ctx context.Context, groupIDs []int64, from, to time.Time) ([]domain.ClassEvent, error) {
	rows, err := s.q.Query(ctx, `
		SELECT id, group_id, title, starts_at, ends_at FROM class_events
		WHERE group_id = ANY($1) AND starts_at < $3 AND ends_at > $2
		ORDER BY starts_at, id`, groupIDs, from, to)
	if err != nil {
		return nil, mapErr(err, "занятия")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.ClassEvent, error) {
		var c domain.ClassEvent
		err := r.Scan(&c.ID, &c.GroupID, &c.Title, &c.StartsAt, &c.EndsAt)
		return c, err
	})
	return res, mapErr(err, "занятия")
}
