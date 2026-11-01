package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

const userColumns = `id, email, password_hash, full_name, role, study_group_id, created_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &u.StudyGroupID, &u.CreatedAt)
	return u, err
}

// CreateUser сохраняет пользователя и возвращает его с id.
func (s *Store) CreateUser(ctx context.Context, u domain.User) (domain.User, error) {
	row := s.q.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, role, study_group_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userColumns,
		u.Email, u.PasswordHash, u.FullName, u.Role, u.StudyGroupID)
	created, err := scanUser(row)
	return created, mapErr(err, "пользователь")
}

// UserByEmail ищет пользователя по e-mail без учёта регистра.
func (s *Store) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := scanUser(s.q.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email))
	return u, mapErr(err, "пользователь")
}

// UserByID возвращает пользователя по id.
func (s *Store) UserByID(ctx context.Context, id int64) (domain.User, error) {
	u, err := scanUser(s.q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	return u, mapErr(err, "пользователь")
}

// ListUsers возвращает всех пользователей по алфавиту.
func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.q.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY full_name, id`)
	if err != nil {
		return nil, mapErr(err, "пользователи")
	}
	users, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.User, error) { return scanUser(r) })
	return users, mapErr(err, "пользователи")
}

// SetUserRole меняет роль пользователя.
func (s *Store) SetUserRole(ctx context.Context, id int64, role domain.Role) error {
	tag, err := s.q.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, id, role)
	return expectOne(tag, err, "пользователь")
}

// SetUserGroup меняет учебную группу пользователя.
func (s *Store) SetUserGroup(ctx context.Context, id int64, groupID *int64) error {
	tag, err := s.q.Exec(ctx, `UPDATE users SET study_group_id = $2 WHERE id = $1`, id, groupID)
	return expectOne(tag, err, "пользователь")
}

// SaveToken сохраняет хэш токена доступа.
func (s *Store) SaveToken(ctx context.Context, hash []byte, userID int64, expiresAt time.Time) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO auth_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hash, userID, expiresAt)
	return mapErr(err, "токен")
}

// UserByToken возвращает владельца действующего токена.
func (s *Store) UserByToken(ctx context.Context, hash []byte, now time.Time) (domain.User, error) {
	u, err := scanUser(s.q.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.full_name, u.role, u.study_group_id, u.created_at
		FROM auth_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1 AND t.expires_at > $2`, hash, now))
	return u, mapErr(err, "токен")
}

// DeleteToken отзывает токен.
func (s *Store) DeleteToken(ctx context.Context, hash []byte) error {
	_, err := s.q.Exec(ctx, `DELETE FROM auth_tokens WHERE token_hash = $1`, hash)
	return mapErr(err, "токен")
}

// DeleteExpiredTokens удаляет истёкшие токены, возвращает их число.
func (s *Store) DeleteExpiredTokens(ctx context.Context, now time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx, `DELETE FROM auth_tokens WHERE expires_at <= $1`, now)
	if err != nil {
		return 0, mapErr(err, "токены")
	}
	return tag.RowsAffected(), nil
}

// GroupByCode ищет учебную группу по коду.
func (s *Store) GroupByCode(ctx context.Context, code string) (domain.StudyGroup, error) {
	var g domain.StudyGroup
	err := s.q.QueryRow(ctx, `SELECT id, code FROM study_groups WHERE code = $1`, code).Scan(&g.ID, &g.Code)
	return g, mapErr(err, "учебная группа")
}

// EnsureGroup возвращает группу с кодом code, создавая её при необходимости.
func (s *Store) EnsureGroup(ctx context.Context, code string) (domain.StudyGroup, error) {
	var g domain.StudyGroup
	err := s.q.QueryRow(ctx, `
		INSERT INTO study_groups (code) VALUES ($1)
		ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
		RETURNING id, code`, code).Scan(&g.ID, &g.Code)
	return g, mapErr(err, "учебная группа")
}
