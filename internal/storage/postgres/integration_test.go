//go:build integration

// Интеграционные тесты на настоящей PostgreSQL:
//
//	TEST_DATABASE_URL=postgres://... go test -tags=integration ./internal/storage/...
//
// Каждый тест работает в своей временной схеме, данные разработки не затрагиваются.
package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/scheduler"
	"github.com/Hamudahabibi/team-scheduler/internal/service"
	"github.com/Hamudahabibi/team-scheduler/internal/storage/postgres"
)

var msk, _ = time.LoadLocation("Europe/Moscow")

// newStore создаёт схему test_<случайно>, применяет миграции и удаляет схему после теста.
func newStore(t *testing.T) *postgres.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx := context.Background()

	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "test_" + hex.EncodeToString(b[:])

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := postgres.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return postgres.NewStore(pool)
}

func TestUsersAndTokens(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	g, err := s.EnsureGroup(ctx, "ИДБ-22-04")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.EnsureGroup(ctx, "ИДБ-22-04"); again.ID != g.ID {
		t.Error("EnsureGroup создал дубль группы")
	}

	u, err := s.CreateUser(ctx, domain.User{Email: "Player@Stankin.ru", PasswordHash: "x", FullName: "Игрок", Role: domain.RolePlayer, StudyGroupID: &g.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, domain.User{Email: "player@stankin.ru", PasswordHash: "x", FullName: "Дубль", Role: domain.RolePlayer}); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("e-mail в другом регистре: %v", err)
	}
	if got, err := s.UserByEmail(ctx, "PLAYER@stankin.ru"); err != nil || got.ID != u.ID {
		t.Errorf("поиск без учёта регистра: %v, %v", got, err)
	}

	now := time.Now()
	hash := []byte("0123456789abcdef0123456789abcdef")
	if err := s.SaveToken(ctx, hash, u.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.UserByToken(ctx, hash, now); err != nil || got.ID != u.ID {
		t.Errorf("UserByToken: %v, %v", got, err)
	}
	if _, err := s.UserByToken(ctx, hash, now.Add(2*time.Hour)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("истёкший токен: %v", err)
	}
	if n, err := s.DeleteExpiredTokens(ctx, now.Add(2*time.Hour)); err != nil || n != 1 {
		t.Errorf("очистка токенов: %d, %v", n, err)
	}
	if _, err := s.UserByID(ctx, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("несуществующий пользователь: %v", err)
	}
}

func TestReplaceClassesIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	g, _ := s.EnsureGroup(ctx, "ИДБ-23-01")

	from := time.Date(2026, 11, 2, 0, 0, 0, 0, msk)
	to := from.AddDate(0, 0, 7)
	classes := []domain.ClassEvent{
		{Title: "Матан", StartsAt: from.Add(8*time.Hour + 30*time.Minute), EndsAt: from.Add(10*time.Hour + 10*time.Minute)},
		{Title: "Физика", StartsAt: from.Add(24*time.Hour + 10*time.Hour), EndsAt: from.Add(24*time.Hour + 12*time.Hour)},
	}
	for range 2 {
		if err := s.ReplaceClasses(ctx, g.ID, from, to, classes); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ClassesByGroups(ctx, []int64{g.ID}, from, to)
	if err != nil || len(got) != 2 {
		t.Fatalf("занятий %d после двух импортов, ошибка %v", len(got), err)
	}
	// Занятие следующей недели не удаляется импортом этой.
	next := []domain.ClassEvent{{Title: "Матан", StartsAt: to.Add(9 * time.Hour), EndsAt: to.Add(10 * time.Hour)}}
	if err := s.ReplaceClasses(ctx, g.ID, to, to.AddDate(0, 0, 7), next); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceClasses(ctx, g.ID, from, to, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ClassesByGroups(ctx, []int64{g.ID}, from, to.AddDate(0, 0, 7))
	if len(got) != 1 || got[0].Title != "Матан" || !got[0].StartsAt.Equal(next[0].StartsAt) {
		t.Errorf("после очистки недели: %+v", got)
	}
	if err := s.ReplaceClasses(ctx, 999, from, to, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("несуществующая группа: %v", err)
	}
}

// fixture — киберцентр с одним залом на 5 мест и двумя командами,
// которые могут тренироваться только в одно и то же время.
type fixture struct {
	store    *postgres.Store
	planning *service.Planning
	admin    domain.Actor
	teamA    int64
	teamB    int64
	week     time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	f := fixture{store: s, week: time.Date(2026, 11, 9, 0, 0, 0, 0, msk)}

	adminUser, err := s.CreateUser(ctx, domain.User{Email: "admin@x.ru", PasswordHash: "x", FullName: "Админ", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	f.admin = domain.Actor{UserID: adminUser.ID, Role: domain.RoleAdmin}

	room, err := s.CreateRoom(ctx, domain.Room{Name: "Зал", Seats: 5})
	if err != nil {
		t.Fatal(err)
	}
	var hours []domain.RoomHours
	for d := 1; d <= 7; d++ {
		hours = append(hours, domain.RoomHours{Weekday: d, OpenMin: 10 * 60, CloseMin: 22 * 60})
	}
	if err := s.ReplaceRoomHours(ctx, room.ID, hours); err != nil {
		t.Fatal(err)
	}

	for i, name := range []string{"A", "B"} {
		team, err := s.CreateTeam(ctx, domain.Team{Name: name, DisciplineID: 1})
		if err != nil {
			t.Fatal(err)
		}
		for j := range 5 {
			u, err := s.CreateUser(ctx, domain.User{
				Email: name + string(rune('0'+j)) + "@x.ru", PasswordHash: "x", FullName: name, Role: domain.RolePlayer,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.UpsertMember(ctx, domain.TeamMember{TeamID: team.ID, UserID: u.ID, Role: domain.MemberCore}); err != nil {
				t.Fatal(err)
			}
			// Все свободны только в понедельник 18:00–20:00.
			if err := s.ReplaceRules(ctx, u.ID, []domain.AvailabilityRule{{Weekday: 1, StartMin: 18 * 60, EndMin: 20 * 60, Kind: domain.RuleAvailable}}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.UpsertPlan(ctx, domain.TrainingPlan{
			TeamID: team.ID, SessionsPerWeek: 1, DurationMin: 120, MinCorePlayers: 5,
			DayStartMin: 12 * 60, DayEndMin: 22 * 60, Priority: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			f.teamA = team.ID
		} else {
			f.teamB = team.ID
		}
	}

	f.planning = service.NewPlanning(s, service.PlanningConfig{Loc: msk, Buffer: 10 * time.Minute, TimeLimit: time.Second})
	return f
}

func (f fixture) generate(t *testing.T, teamIDs ...int64) domain.ScheduleRun {
	t.Helper()
	run, err := f.planning.Generate(context.Background(), f.admin, service.GenerateInput{
		WeekStart: f.week, Algorithm: scheduler.Greedy, TeamIDs: teamIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestGenerateRespectsPublishedSessions(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	runA := f.generate(t, f.teamA)
	if len(runA.Sessions) != 1 || runA.Status != domain.RunDraft {
		t.Fatalf("черновик A: %+v", runA)
	}
	if _, err := f.planning.Publish(ctx, f.admin, runA.ID); err != nil {
		t.Fatal(err)
	}

	// Зал на 5 мест уже занят командой A — для B места нет.
	runB := f.generate(t, f.teamB)
	if len(runB.Sessions) != 0 {
		t.Errorf("B поставлена в занятый зал: %+v", runB.Sessions)
	}

	// Перепланирование A: её собственная тренировка не мешает, старая отменяется.
	runA2 := f.generate(t, f.teamA)
	if len(runA2.Sessions) != 1 {
		t.Fatalf("перепланирование A: %+v", runA2.Sessions)
	}
	if _, err := f.planning.Publish(ctx, f.admin, runA2.ID); err != nil {
		t.Fatal(err)
	}
	published, err := f.planning.WeekSchedule(ctx, f.week, f.teamA)
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 || published[0].RunID == nil || *published[0].RunID != runA2.ID {
		t.Errorf("опубликовано после перепланирования: %+v", published)
	}
	if _, err := f.planning.Publish(ctx, f.admin, runA2.ID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("повторная публикация: %v", err)
	}
}

func TestPublishDetectsConflict(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	// Оба черновика сгенерированы, пока зал свободен, и претендуют на одно время.
	runA := f.generate(t, f.teamA)
	runB := f.generate(t, f.teamB)
	if len(runA.Sessions) != 1 || len(runB.Sessions) != 1 {
		t.Fatalf("черновики: %d и %d тренировок", len(runA.Sessions), len(runB.Sessions))
	}

	if _, err := f.planning.Publish(ctx, f.admin, runA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.planning.Publish(ctx, f.admin, runB.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("публикация в занятый зал: %v", err)
	}
	// Откат: черновик B остался черновиком, его тренировки не опубликованы.
	run, _ := f.store.RunByID(ctx, runB.ID)
	if run.Status != domain.RunDraft || run.Sessions[0].Status != domain.SessionDraft {
		t.Errorf("после конфликта: запуск %s, тренировка %s", run.Status, run.Sessions[0].Status)
	}
}

func TestConcurrentPublishIsSerialized(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	runA := f.generate(t, f.teamA)
	runB := f.generate(t, f.teamB)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []int64{runA.ID, runB.ID} {
		wg.Go(func() {
			_, errs[i] = f.planning.Publish(ctx, f.admin, id)
		})
	}
	wg.Wait()

	ok, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrConflict):
			conflicts++
		default:
			t.Errorf("неожиданная ошибка: %v", err)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Errorf("успешных публикаций %d, конфликтов %d; ожидалось 1 и 1", ok, conflicts)
	}
	published, _ := f.planning.WeekSchedule(ctx, f.week, 0)
	if len(published) != 1 {
		t.Errorf("опубликовано тренировок: %d", len(published))
	}
}

func TestRoomBlockMakesPublishFail(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	run := f.generate(t, f.teamA)

	rooms, _ := f.store.ListRooms(ctx)
	if _, err := f.store.CreateRoomBlock(ctx, domain.RoomBlock{
		RoomID: rooms[0].ID, StartsAt: f.week.Add(17 * time.Hour), EndsAt: f.week.Add(21 * time.Hour), Reason: "турнир",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.planning.Publish(ctx, f.admin, run.ID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("публикация на заблокированный зал: %v", err)
	}
}
