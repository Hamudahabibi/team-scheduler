package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// RulesByUsers возвращает еженедельные правила доступности игроков.
func (s *Store) RulesByUsers(ctx context.Context, userIDs []int64) ([]domain.AvailabilityRule, error) {
	rows, err := s.q.Query(ctx, `
		SELECT id, user_id, weekday, start_min, end_min, kind FROM availability_rules
		WHERE user_id = ANY($1) ORDER BY user_id, weekday, start_min`, userIDs)
	if err != nil {
		return nil, mapErr(err, "правила доступности")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.AvailabilityRule, error) {
		var a domain.AvailabilityRule
		err := r.Scan(&a.ID, &a.UserID, &a.Weekday, &a.StartMin, &a.EndMin, &a.Kind)
		return a, err
	})
	return res, mapErr(err, "правила доступности")
}

// ReplaceRules заменяет правила доступности игрока целиком.
func (s *Store) ReplaceRules(ctx context.Context, userID int64, rules []domain.AvailabilityRule) error {
	return s.InTx(ctx, func(tx *Store) error {
		if _, err := tx.q.Exec(ctx, `DELETE FROM availability_rules WHERE user_id = $1`, userID); err != nil {
			return mapErr(err, "правила доступности")
		}
		_, err := tx.q.CopyFrom(ctx, pgx.Identifier{"availability_rules"},
			[]string{"user_id", "weekday", "start_min", "end_min", "kind"},
			pgx.CopyFromSlice(len(rules), func(i int) ([]any, error) {
				r := rules[i]
				return []any{userID, r.Weekday, r.StartMin, r.EndMin, r.Kind}, nil
			}))
		return mapErr(err, "правила доступности")
	})
}

// ExceptionsByUsers возвращает исключения игроков, пересекающие [from, to).
func (s *Store) ExceptionsByUsers(ctx context.Context, userIDs []int64, from, to time.Time) ([]domain.AvailabilityException, error) {
	rows, err := s.q.Query(ctx, `
		SELECT id, user_id, starts_at, ends_at, kind, note FROM availability_exceptions
		WHERE user_id = ANY($1) AND starts_at < $3 AND ends_at > $2
		ORDER BY starts_at, id`, userIDs, from, to)
	if err != nil {
		return nil, mapErr(err, "исключения доступности")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.AvailabilityException, error) {
		var e domain.AvailabilityException
		err := r.Scan(&e.ID, &e.UserID, &e.StartsAt, &e.EndsAt, &e.Kind, &e.Note)
		return e, err
	})
	return res, mapErr(err, "исключения доступности")
}

// CreateException сохраняет исключение доступности.
func (s *Store) CreateException(ctx context.Context, e domain.AvailabilityException) (domain.AvailabilityException, error) {
	err := s.q.QueryRow(ctx, `
		INSERT INTO availability_exceptions (user_id, starts_at, ends_at, kind, note)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		e.UserID, e.StartsAt, e.EndsAt, e.Kind, e.Note).Scan(&e.ID)
	return e, mapErr(err, "исключение доступности")
}

// DeleteException удаляет исключение игрока.
func (s *Store) DeleteException(ctx context.Context, userID, id int64) error {
	tag, err := s.q.Exec(ctx, `DELETE FROM availability_exceptions WHERE id = $1 AND user_id = $2`, id, userID)
	return expectOne(tag, err, "исключение доступности")
}
