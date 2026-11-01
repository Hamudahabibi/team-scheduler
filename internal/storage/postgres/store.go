package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// querier — общее подмножество методов пула и транзакции.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, src pgx.CopyFromSource) (int64, error)
}

// Store — репозиторий всех сущностей. Один и тот же тип работает
// и поверх пула, и внутри транзакции (см. InTx).
type Store struct {
	pool *pgxpool.Pool
	q    querier
}

// NewStore создаёт хранилище поверх пула соединений.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: pool}
}

// Ping проверяет соединение с БД.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// InTx выполняет fn в транзакции. Если fn вернула ошибку, транзакция
// откатывается. Вложенный вызов переиспользует текущую транзакцию.
func (s *Store) InTx(ctx context.Context, fn func(tx *Store) error) error {
	if _, ok := s.q.(pgx.Tx); ok {
		return fn(s)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(&Store{pool: s.pool, q: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("фиксация транзакции: %w", err)
	}
	return nil
}

// Коды ошибок PostgreSQL, которые переводятся в доменные.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgCheckViolation      = "23514"
)

// mapErr переводит ошибки драйвера в доменные, чтобы сервисы
// не зависели от pgx.
func mapErr(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, domain.ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			return fmt.Errorf("%s уже существует: %w", what, domain.ErrConflict)
		case pgForeignKeyViolation:
			return fmt.Errorf("%s: связанная запись не найдена или используется: %w", what, domain.ErrConflict)
		case pgCheckViolation:
			return fmt.Errorf("%s: нарушено ограничение %s: %w", what, pgErr.ConstraintName, domain.ErrValidation)
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}

// expectOne проверяет, что запрос изменил ровно одну строку.
func expectOne(tag pgconn.CommandTag, err error, what string) error {
	if err != nil {
		return mapErr(err, what)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", what, domain.ErrNotFound)
	}
	return nil
}
