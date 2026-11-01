// Package service содержит сценарии использования: проверку прав,
// валидацию и координацию хранилища с алгоритмами планирования.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// AuthRepo — хранилище, нужное сервису учётных записей.
type AuthRepo interface {
	CreateUser(ctx context.Context, u domain.User) (domain.User, error)
	UserByEmail(ctx context.Context, email string) (domain.User, error)
	UserByID(ctx context.Context, id int64) (domain.User, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	SetUserRole(ctx context.Context, id int64, role domain.Role) error
	SetUserGroup(ctx context.Context, id int64, groupID *int64) error
	SaveToken(ctx context.Context, hash []byte, userID int64, expiresAt time.Time) error
	UserByToken(ctx context.Context, hash []byte, now time.Time) (domain.User, error)
	DeleteToken(ctx context.Context, hash []byte) error
	EnsureGroup(ctx context.Context, code string) (domain.StudyGroup, error)
}

// Auth — регистрация, вход и управление пользователями.
type Auth struct {
	repo       AuthRepo
	tokenTTL   time.Duration
	adminEmail string
	now        func() time.Time
	// dummyHash сравнивается с паролем, если пользователь не найден,
	// чтобы время ответа не выдавало существование e-mail.
	dummyHash []byte
}

// NewAuth создаёт сервис. adminEmail — e-mail, который при регистрации
// сразу получает роль администратора (первичная настройка киберцентра).
func NewAuth(repo AuthRepo, tokenTTL time.Duration, adminEmail string) *Auth {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)
	return &Auth{
		repo:       repo,
		tokenTTL:   tokenTTL,
		adminEmail: strings.ToLower(strings.TrimSpace(adminEmail)),
		now:        time.Now,
		dummyHash:  dummy,
	}
}

// RegisterInput — данные для самостоятельной регистрации игрока.
type RegisterInput struct {
	Email     string
	Password  string
	FullName  string
	GroupCode string
}

const (
	minPasswordLen = 8
	// bcrypt учитывает только первые 72 байта пароля.
	maxPasswordBytes = 72
)

// Register создаёт игрока. Учебная группа создаётся, если её ещё нет:
// администратор загрузит её расписание позже.
func (a *Auth) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	email := strings.TrimSpace(in.Email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return domain.User{}, domain.Invalid("email", "некорректный e-mail")
	}
	if utf8.RuneCountInString(in.Password) < minPasswordLen {
		return domain.User{}, domain.Invalid("password", fmt.Sprintf("пароль короче %d символов", minPasswordLen))
	}
	if len(in.Password) > maxPasswordBytes {
		return domain.User{}, domain.Invalid("password", "пароль слишком длинный")
	}
	fullName := strings.TrimSpace(in.FullName)
	if fullName == "" {
		return domain.User{}, domain.Invalid("full_name", "укажите ФИО")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("хэширование пароля: %w", err)
	}

	u := domain.User{
		Email:        email,
		PasswordHash: string(hash),
		FullName:     fullName,
		Role:         domain.RolePlayer,
	}
	if a.adminEmail != "" && strings.ToLower(email) == a.adminEmail {
		u.Role = domain.RoleAdmin
	}
	if code := normalizeGroupCode(in.GroupCode); code != "" {
		g, err := a.repo.EnsureGroup(ctx, code)
		if err != nil {
			return domain.User{}, err
		}
		u.StudyGroupID = &g.ID
	}
	return a.repo.CreateUser(ctx, u)
}

// Login проверяет пароль и выдаёт новый токен доступа.
func (a *Auth) Login(ctx context.Context, email, password string) (string, domain.User, error) {
	u, err := a.repo.UserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(a.dummyHash, []byte(password))
			return "", domain.User{}, fmt.Errorf("неверный e-mail или пароль: %w", domain.ErrUnauthorized)
		}
		return "", domain.User{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", domain.User{}, fmt.Errorf("неверный e-mail или пароль: %w", domain.ErrUnauthorized)
	}

	token, hash, err := newToken()
	if err != nil {
		return "", domain.User{}, err
	}
	if err := a.repo.SaveToken(ctx, hash, u.ID, a.now().Add(a.tokenTTL)); err != nil {
		return "", domain.User{}, err
	}
	return token, u, nil
}

// Authenticate возвращает пользователя по токену доступа.
func (a *Auth) Authenticate(ctx context.Context, token string) (domain.User, error) {
	if token == "" {
		return domain.User{}, domain.ErrUnauthorized
	}
	u, err := a.repo.UserByToken(ctx, hashToken(token), a.now())
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, fmt.Errorf("токен недействителен или истёк: %w", domain.ErrUnauthorized)
	}
	return u, err
}

// Logout отзывает токен.
func (a *Auth) Logout(ctx context.Context, token string) error {
	return a.repo.DeleteToken(ctx, hashToken(token))
}

// Me возвращает актуальные данные пользователя.
func (a *Auth) Me(ctx context.Context, actor domain.Actor) (domain.User, error) {
	return a.repo.UserByID(ctx, actor.UserID)
}

// ListUsers доступен администратору и тренерам: тренеру нужно
// находить игроков, чтобы добавлять их в состав.
func (a *Auth) ListUsers(ctx context.Context, actor domain.Actor) ([]domain.User, error) {
	if actor.Role == domain.RolePlayer {
		return nil, domain.ErrForbidden
	}
	return a.repo.ListUsers(ctx)
}

// SetRole меняет роль пользователя. Только для администратора.
func (a *Auth) SetRole(ctx context.Context, actor domain.Actor, userID int64, role domain.Role) error {
	if !actor.IsAdmin() {
		return domain.ErrForbidden
	}
	if !role.Valid() {
		return domain.Invalid("role", "неизвестная роль")
	}
	if userID == actor.UserID && role != domain.RoleAdmin {
		return fmt.Errorf("нельзя снять роль администратора с самого себя: %w", domain.ErrConflict)
	}
	return a.repo.SetUserRole(ctx, userID, role)
}

// SetMyGroup меняет учебную группу текущего пользователя.
// Пустой код отвязывает пользователя от группы.
func (a *Auth) SetMyGroup(ctx context.Context, actor domain.Actor, groupCode string) (domain.User, error) {
	var groupID *int64
	if code := normalizeGroupCode(groupCode); code != "" {
		g, err := a.repo.EnsureGroup(ctx, code)
		if err != nil {
			return domain.User{}, err
		}
		groupID = &g.ID
	}
	if err := a.repo.SetUserGroup(ctx, actor.UserID, groupID); err != nil {
		return domain.User{}, err
	}
	return a.repo.UserByID(ctx, actor.UserID)
}

// normalizeGroupCode приводит код группы к единому виду: «идб-22-04 » → «ИДБ-22-04».
func normalizeGroupCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// newToken генерирует случайный токен и его SHA-256 для хранения в БД.
func newToken() (token string, hash []byte, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, fmt.Errorf("генерация токена: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b[:])
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
