package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// fakeAuthRepo — хранилище в памяти для тестов сервиса учётных записей.
type fakeAuthRepo struct {
	users  []domain.User
	groups map[string]domain.StudyGroup
	tokens map[string]tokenRow
}

type tokenRow struct {
	userID    int64
	expiresAt time.Time
}

func newFakeAuthRepo() *fakeAuthRepo {
	return &fakeAuthRepo{groups: map[string]domain.StudyGroup{}, tokens: map[string]tokenRow{}}
}

func (f *fakeAuthRepo) CreateUser(_ context.Context, u domain.User) (domain.User, error) {
	for _, existing := range f.users {
		if strings.EqualFold(existing.Email, u.Email) {
			return domain.User{}, domain.ErrConflict
		}
	}
	u.ID = int64(len(f.users) + 1)
	f.users = append(f.users, u)
	return u, nil
}

func (f *fakeAuthRepo) UserByEmail(_ context.Context, email string) (domain.User, error) {
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *fakeAuthRepo) UserByID(_ context.Context, id int64) (domain.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *fakeAuthRepo) ListUsers(context.Context) ([]domain.User, error) { return f.users, nil }

func (f *fakeAuthRepo) SetUserRole(_ context.Context, id int64, role domain.Role) error {
	for i := range f.users {
		if f.users[i].ID == id {
			f.users[i].Role = role
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeAuthRepo) SetUserGroup(_ context.Context, id int64, groupID *int64) error {
	for i := range f.users {
		if f.users[i].ID == id {
			f.users[i].StudyGroupID = groupID
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeAuthRepo) SaveToken(_ context.Context, hash []byte, userID int64, exp time.Time) error {
	f.tokens[string(hash)] = tokenRow{userID, exp}
	return nil
}

func (f *fakeAuthRepo) UserByToken(ctx context.Context, hash []byte, now time.Time) (domain.User, error) {
	row, ok := f.tokens[string(hash)]
	if !ok || !row.expiresAt.After(now) {
		return domain.User{}, domain.ErrNotFound
	}
	return f.UserByID(ctx, row.userID)
}

func (f *fakeAuthRepo) DeleteToken(_ context.Context, hash []byte) error {
	delete(f.tokens, string(hash))
	return nil
}

func (f *fakeAuthRepo) EnsureGroup(_ context.Context, code string) (domain.StudyGroup, error) {
	if g, ok := f.groups[code]; ok {
		return g, nil
	}
	g := domain.StudyGroup{ID: int64(len(f.groups) + 1), Code: code}
	f.groups[code] = g
	return g, nil
}

func TestRegisterValidation(t *testing.T) {
	auth := NewAuth(newFakeAuthRepo(), time.Hour, "")
	tests := []struct {
		name  string
		in    RegisterInput
		field string
	}{
		{"плохой e-mail", RegisterInput{Email: "not-email", Password: "password1", FullName: "Иван"}, "email"},
		{"e-mail с именем", RegisterInput{Email: "Иван <a@b.ru>", Password: "password1", FullName: "Иван"}, "email"},
		{"короткий пароль", RegisterInput{Email: "a@b.ru", Password: "1234567", FullName: "Иван"}, "password"},
		{"пустое ФИО", RegisterInput{Email: "a@b.ru", Password: "password1", FullName: "  "}, "full_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Register(context.Background(), tt.in)
			var ve *domain.ValidationError
			if !errors.As(err, &ve) || ve.Field != tt.field {
				t.Fatalf("ожидалась ошибка поля %q, получено %v", tt.field, err)
			}
		})
	}
}

func TestRegisterLoginAuthenticate(t *testing.T) {
	ctx := context.Background()
	repo := newFakeAuthRepo()
	auth := NewAuth(repo, time.Hour, "Admin@Stankin.ru")

	admin, err := auth.Register(ctx, RegisterInput{Email: "admin@stankin.ru", Password: "secret-pass", FullName: "Админ"})
	if err != nil {
		t.Fatal(err)
	}
	if admin.Role != domain.RoleAdmin {
		t.Errorf("bootstrap-администратор получил роль %q", admin.Role)
	}

	player, err := auth.Register(ctx, RegisterInput{
		Email: "player@stankin.ru", Password: "secret-pass", FullName: "Игрок", GroupCode: " идб-22-04 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if player.Role != domain.RolePlayer || player.StudyGroupID == nil {
		t.Fatalf("игрок: роль %q, группа %v", player.Role, player.StudyGroupID)
	}
	if _, ok := repo.groups["ИДБ-22-04"]; !ok {
		t.Error("код группы не нормализован")
	}
	if bytes.Contains([]byte(player.PasswordHash), []byte("secret-pass")) {
		t.Error("пароль хранится в открытом виде")
	}

	if _, err := auth.Register(ctx, RegisterInput{Email: "PLAYER@stankin.ru", Password: "secret-pass", FullName: "Дубль"}); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("повторный e-mail: %v", err)
	}

	if _, _, err := auth.Login(ctx, "player@stankin.ru", "wrong-pass"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("неверный пароль: %v", err)
	}
	if _, _, err := auth.Login(ctx, "nobody@stankin.ru", "secret-pass"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("несуществующий пользователь: %v", err)
	}

	token, _, err := auth.Login(ctx, "player@stankin.ru", "secret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.tokens[token]; ok {
		t.Error("токен хранится в открытом виде")
	}
	got, err := auth.Authenticate(ctx, token)
	if err != nil || got.ID != player.ID {
		t.Fatalf("Authenticate: %v, %v", got, err)
	}

	// Токен истекает.
	auth.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("истёкший токен: %v", err)
	}
	auth.now = time.Now

	if err := auth.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("отозванный токен: %v", err)
	}
}

func TestSetRolePermissions(t *testing.T) {
	ctx := context.Background()
	repo := newFakeAuthRepo()
	auth := NewAuth(repo, time.Hour, "")
	u, _ := auth.Register(ctx, RegisterInput{Email: "p@b.ru", Password: "password1", FullName: "П"})

	player := domain.Actor{UserID: 100, Role: domain.RolePlayer}
	admin := domain.Actor{UserID: 200, Role: domain.RoleAdmin}

	if err := auth.SetRole(ctx, player, u.ID, domain.RoleCoach); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("игрок меняет роль: %v", err)
	}
	if err := auth.SetRole(ctx, admin, u.ID, "boss"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("неизвестная роль: %v", err)
	}
	if err := auth.SetRole(ctx, admin, admin.UserID, domain.RolePlayer); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("админ понижает себя: %v", err)
	}
	if err := auth.SetRole(ctx, admin, u.ID, domain.RoleCoach); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.UserByID(ctx, u.ID); got.Role != domain.RoleCoach {
		t.Errorf("роль не сменилась: %q", got.Role)
	}
}
