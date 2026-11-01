package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/ical"
	"github.com/Hamudahabibi/team-scheduler/internal/scheduler"
	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

// PlanningRepo — хранилище для сервиса планирования.
type PlanningRepo interface {
	availabilitySource
	InTx(ctx context.Context, fn func(ctx context.Context) error) error

	ListTeams(ctx context.Context) ([]domain.Team, error)
	TeamByID(ctx context.Context, id int64) (domain.Team, error)
	TeamsByUser(ctx context.Context, userID int64) ([]domain.Team, error)
	TeamMembers(ctx context.Context, teamID int64) ([]domain.TeamMember, error)
	PlanByTeam(ctx context.Context, teamID int64) (domain.TrainingPlan, error)
	DisciplineByID(ctx context.Context, id int64) (domain.Discipline, error)
	UserByID(ctx context.Context, id int64) (domain.User, error)

	ListRooms(ctx context.Context) ([]domain.Room, error)
	RoomBlocks(ctx context.Context, roomID int64, from, to time.Time) ([]domain.RoomBlock, error)
	LockRooms(ctx context.Context, roomIDs []int64) ([]domain.Room, error)

	CreateRun(ctx context.Context, run domain.ScheduleRun) (domain.ScheduleRun, error)
	RunByID(ctx context.Context, id int64) (domain.ScheduleRun, error)
	LockRun(ctx context.Context, id int64) (domain.ScheduleRun, error)
	RunsByWeek(ctx context.Context, weekStart time.Time) ([]domain.ScheduleRun, error)
	SetRunStatus(ctx context.Context, id int64, status domain.RunStatus, sessionStatus domain.SessionStatus) error

	Sessions(ctx context.Context, f domain.SessionFilter) ([]domain.TrainingSession, error)
	SessionByID(ctx context.Context, id int64) (domain.TrainingSession, error)
	SetSessionStatus(ctx context.Context, id int64, from, to domain.SessionStatus) error
	CancelTeamSessions(ctx context.Context, teamIDs []int64, from, to time.Time) (int64, error)
}

// PlanningConfig — настройки планирования.
type PlanningConfig struct {
	Loc       *time.Location
	Buffer    time.Duration
	TimeLimit time.Duration
	Weights   scheduler.Weights
}

// Planning — генерация, публикация и просмотр расписания тренировок.
type Planning struct {
	repo PlanningRepo
	cfg  PlanningConfig
	now  func() time.Time
}

// NewPlanning создаёт сервис планирования.
func NewPlanning(repo PlanningRepo, cfg PlanningConfig) *Planning {
	if cfg.Weights.Unit == 0 {
		cfg.Weights = scheduler.DefaultWeights()
	}
	return &Planning{repo: repo, cfg: cfg, now: time.Now}
}

// GenerateInput — параметры запуска генерации.
type GenerateInput struct {
	WeekStart time.Time
	Algorithm scheduler.Algorithm
	// TeamIDs — команды для планирования. Пусто — все команды.
	TeamIDs []int64
}

// runStats — статистика запуска, сохраняется в schedule_runs.stats.
type runStats struct {
	scheduler.Stats
	// Skipped — команды, которые не участвовали в генерации, и почему.
	Skipped []scheduler.Shortfall `json:"skipped"`
}

// Generate составляет черновик расписания на неделю. Только администратор:
// залы общие, и решение затрагивает все команды.
func (s *Planning) Generate(ctx context.Context, actor domain.Actor, in GenerateInput) (domain.ScheduleRun, error) {
	if !actor.IsAdmin() {
		return domain.ScheduleRun{}, domain.ErrForbidden
	}
	if in.Algorithm == "" {
		in.Algorithm = scheduler.Greedy
	}
	if in.Algorithm != scheduler.Greedy && in.Algorithm != scheduler.BranchAndBound {
		return domain.ScheduleRun{}, domain.Invalid("algorithm", "greedy или bnb")
	}
	w, err := s.week(in.WeekStart)
	if err != nil {
		return domain.ScheduleRun{}, err
	}
	teams, err := s.selectTeams(ctx, in.TeamIDs)
	if err != nil {
		return domain.ScheduleRun{}, err
	}

	problem, skipped, err := s.buildProblem(ctx, w, teams)
	if err != nil {
		return domain.ScheduleRun{}, err
	}
	res, err := scheduler.Solve(ctx, problem, scheduler.Options{Algorithm: in.Algorithm, TimeLimit: s.cfg.TimeLimit})
	if err != nil {
		return domain.ScheduleRun{}, fmt.Errorf("планирование: %w", err)
	}

	stats, err := json.Marshal(runStats{Stats: res.Stats, Skipped: skipped})
	if err != nil {
		return domain.ScheduleRun{}, fmt.Errorf("статистика: %w", err)
	}
	run := domain.ScheduleRun{
		WeekStart: dateOnly(w.Start()),
		Algorithm: string(in.Algorithm),
		Objective: res.Stats.Objective,
		Stats:     stats,
		CreatedBy: &actor.UserID,
	}
	for _, t := range teams {
		run.TeamIDs = append(run.TeamIDs, t.ID)
	}
	for _, ss := range res.Sessions {
		run.Sessions = append(run.Sessions, domain.TrainingSession{
			TeamID:             ss.TeamID,
			RoomID:             ss.RoomID,
			StartsAt:           w.SlotTime(ss.Start),
			EndsAt:             w.SlotTime(ss.Start + ss.Duration),
			Seats:              ss.Seats,
			Quality:            ss.Quality,
			ExpectedAttendance: ss.Attendance,
		})
	}
	return s.repo.CreateRun(ctx, run)
}

func (s *Planning) week(start time.Time) (timegrid.Week, error) {
	w, err := timegrid.NewWeek(start, s.cfg.Loc)
	if err != nil {
		return w, domain.Invalid("week_start", err.Error())
	}
	return w, nil
}

// dateOnly — дата как полночь UTC: так она без сдвигов ложится в столбец DATE.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *Planning) selectTeams(ctx context.Context, ids []int64) ([]domain.Team, error) {
	if len(ids) == 0 {
		return s.repo.ListTeams(ctx)
	}
	teams := make([]domain.Team, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		t, err := s.repo.TeamByID(ctx, id)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.Invalid("team_ids", fmt.Sprintf("команда %d не найдена", id))
		}
		if err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, nil
}

// buildProblem переводит данные из БД в задачу для планировщика.
func (s *Planning) buildProblem(ctx context.Context, w timegrid.Week, teams []domain.Team) (scheduler.Problem, []scheduler.Shortfall, error) {
	problem := scheduler.Problem{Weights: s.cfg.Weights}
	skipped := []scheduler.Shortfall{}
	planned := map[int64]bool{}

	type teamData struct {
		team    domain.Team
		plan    domain.TrainingPlan
		members []domain.TeamMember
		seats   int
	}
	var data []teamData
	var refs []playerRef
	disciplines := map[int64]domain.Discipline{}

	for _, t := range teams {
		plan, err := s.repo.PlanByTeam(ctx, t.ID)
		if errors.Is(err, domain.ErrNotFound) {
			skipped = append(skipped, scheduler.Shortfall{TeamID: t.ID, Reason: "у команды нет тренировочного плана"})
			continue
		}
		if err != nil {
			return problem, nil, err
		}
		members, err := s.repo.TeamMembers(ctx, t.ID)
		if err != nil {
			return problem, nil, err
		}
		d, ok := disciplines[t.DisciplineID]
		if !ok {
			if d, err = s.repo.DisciplineByID(ctx, t.DisciplineID); err != nil {
				return problem, nil, err
			}
			disciplines[t.DisciplineID] = d
		}
		for _, m := range members {
			refs = append(refs, playerRef{UserID: m.UserID, GroupID: m.StudyGroupID})
		}
		planned[t.ID] = true
		data = append(data, teamData{team: t, plan: plan, members: members, seats: d.RosterSize})
	}

	masks, err := loadPlayerMasks(ctx, s.repo, w, refs, s.cfg.Buffer)
	if err != nil {
		return problem, nil, err
	}
	for _, td := range data {
		st := scheduler.Team{
			ID:       td.team.ID,
			Sessions: td.plan.SessionsPerWeek,
			Duration: td.plan.DurationMin / timegrid.SlotMinutes,
			MinCore:  td.plan.MinCorePlayers,
			DayStart: td.plan.DayStartMin / timegrid.SlotMinutes,
			DayEnd:   td.plan.DayEndMin / timegrid.SlotMinutes,
			MinRest:  (td.plan.MinRestMin + timegrid.SlotMinutes - 1) / timegrid.SlotMinutes,
			Priority: td.plan.Priority,
			Seats:    td.seats,
		}
		for _, m := range td.members {
			pm := masks[m.UserID]
			st.Members = append(st.Members, scheduler.Member{
				UserID: m.UserID, Core: m.Role == domain.MemberCore, Free: pm.Free, Preferred: pm.Preferred,
			})
		}
		problem.Teams = append(problem.Teams, st)
	}

	rooms, err := s.roomCapacity(ctx, w, planned)
	if err != nil {
		return problem, nil, err
	}
	problem.Rooms = rooms
	return problem, skipped, nil
}

// roomCapacity считает свободные места в каждом слоте недели: часы работы,
// минус блокировки, минус опубликованные тренировки команд, которые сейчас
// не перепланируются.
func (s *Planning) roomCapacity(ctx context.Context, w timegrid.Week, replanned map[int64]bool) ([]scheduler.Room, error) {
	rooms, err := s.repo.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	blocks, err := s.repo.RoomBlocks(ctx, 0, w.Start(), w.End())
	if err != nil {
		return nil, err
	}
	published, err := s.repo.Sessions(ctx, domain.SessionFilter{From: w.Start(), To: w.End(), Status: domain.SessionPublished})
	if err != nil {
		return nil, err
	}

	res := make([]scheduler.Room, 0, len(rooms))
	for _, r := range rooms {
		sr := scheduler.Room{ID: r.ID}
		for _, h := range r.Hours {
			open := timegrid.Weekly(h.Weekday, h.OpenMin, h.CloseMin)
			for slot := range timegrid.SlotsPerWeek {
				if open.Has(slot) {
					sr.Free[slot] = r.Seats
				}
			}
		}
		for _, b := range blocks {
			if b.RoomID != r.ID {
				continue
			}
			busy := w.Cover(b.StartsAt, b.EndsAt)
			for slot := range timegrid.SlotsPerWeek {
				if busy.Has(slot) {
					sr.Free[slot] = 0
				}
			}
		}
		for _, ts := range published {
			if ts.RoomID != r.ID || replanned[ts.TeamID] {
				continue
			}
			busy := w.Cover(ts.StartsAt, ts.EndsAt)
			for slot := range timegrid.SlotsPerWeek {
				if busy.Has(slot) {
					sr.Free[slot] = max(sr.Free[slot]-ts.Seats, 0)
				}
			}
		}
		res = append(res, sr)
	}
	return res, nil
}

// Publish публикует черновик в одной транзакции (docs/architecture.md, 4.3):
// блокирует залы, отменяет прежние тренировки команд запуска на эту неделю
// и перепроверяет вместимость и блокировки — за время между генерацией
// и публикацией могли опубликовать другой запуск или закрыть зал.
func (s *Planning) Publish(ctx context.Context, actor domain.Actor, runID int64) (domain.ScheduleRun, error) {
	if !actor.IsAdmin() {
		return domain.ScheduleRun{}, domain.ErrForbidden
	}
	err := s.repo.InTx(ctx, func(ctx context.Context) error {
		run, err := s.repo.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		if run.Status != domain.RunDraft {
			return fmt.Errorf("запуск уже в статусе %s: %w", run.Status, domain.ErrConflict)
		}
		full, err := s.repo.RunByID(ctx, runID)
		if err != nil {
			return err
		}
		w, err := s.runWeek(run)
		if err != nil {
			return err
		}

		roomIDs := make([]int64, 0)
		for _, ts := range full.Sessions {
			if !slices.Contains(roomIDs, ts.RoomID) {
				roomIDs = append(roomIDs, ts.RoomID)
			}
		}
		rooms, err := s.repo.LockRooms(ctx, roomIDs)
		if err != nil {
			return err
		}
		if _, err := s.repo.CancelTeamSessions(ctx, run.TeamIDs, w.Start(), w.End()); err != nil {
			return err
		}
		if err := s.checkPublish(ctx, w, rooms, full.Sessions); err != nil {
			return err
		}
		return s.repo.SetRunStatus(ctx, runID, domain.RunPublished, domain.SessionPublished)
	})
	if err != nil {
		return domain.ScheduleRun{}, err
	}
	return s.repo.RunByID(ctx, runID)
}

func (s *Planning) runWeek(run domain.ScheduleRun) (timegrid.Week, error) {
	y, m, d := run.WeekStart.Date()
	return timegrid.NewWeek(time.Date(y, m, d, 0, 0, 0, 0, s.cfg.Loc), s.cfg.Loc)
}

// checkPublish проверяет, что черновик помещается в залы вместе с уже
// опубликованными тренировками и не попадает на блокировки.
func (s *Planning) checkPublish(ctx context.Context, w timegrid.Week, rooms []domain.Room, drafts []domain.TrainingSession) error {
	if len(drafts) == 0 {
		return nil
	}
	roomIDs := make([]int64, len(rooms))
	seats := map[int64]int{}
	names := map[int64]string{}
	for i, r := range rooms {
		roomIDs[i] = r.ID
		seats[r.ID] = r.Seats
		names[r.ID] = r.Name
	}
	published, err := s.repo.Sessions(ctx, domain.SessionFilter{
		From: w.Start(), To: w.End(), Status: domain.SessionPublished, RoomIDs: roomIDs,
	})
	if err != nil {
		return err
	}
	blocks, err := s.repo.RoomBlocks(ctx, 0, w.Start(), w.End())
	if err != nil {
		return err
	}

	used := map[int64]*[timegrid.SlotsPerWeek]int{}
	add := func(ts domain.TrainingSession) (int, bool) {
		u := used[ts.RoomID]
		if u == nil {
			u = new([timegrid.SlotsPerWeek]int)
			used[ts.RoomID] = u
		}
		busy := w.Cover(ts.StartsAt, ts.EndsAt)
		for slot := range timegrid.SlotsPerWeek {
			if busy.Has(slot) {
				u[slot] += ts.Seats
				if u[slot] > seats[ts.RoomID] {
					return slot, false
				}
			}
		}
		return 0, true
	}
	for _, ts := range published {
		add(ts)
	}
	for _, ts := range drafts {
		for _, b := range blocks {
			if b.RoomID == ts.RoomID && b.StartsAt.Before(ts.EndsAt) && ts.StartsAt.Before(b.EndsAt) {
				return fmt.Errorf("зал %q заблокирован %s (%s): %w",
					names[ts.RoomID], b.StartsAt.In(s.cfg.Loc).Format("02.01 15:04"), b.Reason, domain.ErrConflict)
			}
		}
		if slot, ok := add(ts); !ok {
			return fmt.Errorf("в зале %q не хватает мест %s, сгенерируйте расписание заново: %w",
				names[ts.RoomID], w.SlotTime(slot).Format("02.01 15:04"), domain.ErrConflict)
		}
	}
	return nil
}

// Discard отклоняет черновик.
func (s *Planning) Discard(ctx context.Context, actor domain.Actor, runID int64) error {
	if !actor.IsAdmin() {
		return domain.ErrForbidden
	}
	return s.repo.InTx(ctx, func(ctx context.Context) error {
		run, err := s.repo.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		if run.Status != domain.RunDraft {
			return fmt.Errorf("отклонить можно только черновик, статус %s: %w", run.Status, domain.ErrConflict)
		}
		return s.repo.SetRunStatus(ctx, runID, domain.RunDiscarded, domain.SessionCancelled)
	})
}

// Run возвращает запуск с тренировками.
func (s *Planning) Run(ctx context.Context, id int64) (domain.ScheduleRun, error) {
	return s.repo.RunByID(ctx, id)
}

// Runs возвращает запуски генерации на неделю.
func (s *Planning) Runs(ctx context.Context, weekStart time.Time) ([]domain.ScheduleRun, error) {
	w, err := s.week(weekStart)
	if err != nil {
		return nil, err
	}
	return s.repo.RunsByWeek(ctx, dateOnly(w.Start()))
}

// WeekSchedule возвращает опубликованные тренировки недели. teamID = 0 — все команды.
func (s *Planning) WeekSchedule(ctx context.Context, weekStart time.Time, teamID int64) ([]domain.TrainingSession, error) {
	w, err := s.week(weekStart)
	if err != nil {
		return nil, err
	}
	f := domain.SessionFilter{From: w.Start(), To: w.End(), Status: domain.SessionPublished}
	if teamID > 0 {
		f.TeamIDs = []int64{teamID}
	}
	return s.repo.Sessions(ctx, f)
}

// MySchedule — неделя игрока: тренировки его команд и пары его группы.
type MySchedule struct {
	WeekStart time.Time                `json:"week_start"`
	Trainings []domain.TrainingSession `json:"trainings"`
	Classes   []domain.ClassEvent      `json:"classes"`
}

// MyWeek возвращает расписание текущего пользователя на неделю.
func (s *Planning) MyWeek(ctx context.Context, actor domain.Actor, weekStart time.Time) (MySchedule, error) {
	w, err := s.week(weekStart)
	if err != nil {
		return MySchedule{}, err
	}
	res := MySchedule{WeekStart: w.Start(), Trainings: []domain.TrainingSession{}, Classes: []domain.ClassEvent{}}

	teams, err := s.repo.TeamsByUser(ctx, actor.UserID)
	if err != nil {
		return res, err
	}
	if len(teams) > 0 {
		ids := make([]int64, len(teams))
		for i, t := range teams {
			ids[i] = t.ID
		}
		res.Trainings, err = s.repo.Sessions(ctx, domain.SessionFilter{
			From: w.Start(), To: w.End(), Status: domain.SessionPublished, TeamIDs: ids,
		})
		if err != nil {
			return res, err
		}
	}

	u, err := s.repo.UserByID(ctx, actor.UserID)
	if err != nil {
		return res, err
	}
	if u.StudyGroupID != nil {
		res.Classes, err = s.repo.ClassesByGroups(ctx, []int64{*u.StudyGroupID}, w.Start(), w.End())
	}
	return res, err
}

// CancelSession отменяет опубликованную тренировку. Администратор или тренер команды.
func (s *Planning) CancelSession(ctx context.Context, actor domain.Actor, id int64) error {
	ts, err := s.repo.SessionByID(ctx, id)
	if err != nil {
		return err
	}
	t, err := s.repo.TeamByID(ctx, ts.TeamID)
	if err != nil {
		return err
	}
	if !canManageTeam(actor, t) {
		return domain.ErrForbidden
	}
	if ts.Status != domain.SessionPublished {
		return fmt.Errorf("отменить можно только опубликованную тренировку: %w", domain.ErrConflict)
	}
	return s.repo.SetSessionStatus(ctx, id, domain.SessionPublished, domain.SessionCancelled)
}

// TeamCalendar выгружает опубликованные тренировки команды в iCalendar.
func (s *Planning) TeamCalendar(ctx context.Context, teamID int64, from, to time.Time) ([]byte, error) {
	if !to.After(from) {
		return nil, domain.Invalid("to", "конец периода должен быть позже начала")
	}
	if to.Sub(from) > maxImportPeriod {
		return nil, domain.Invalid("to", "период не больше года")
	}
	t, err := s.repo.TeamByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.repo.Sessions(ctx, domain.SessionFilter{
		From: from, To: to, Status: domain.SessionPublished, TeamIDs: []int64{teamID},
	})
	if err != nil {
		return nil, err
	}
	events := make([]ical.ExportEvent, len(sessions))
	for i, ts := range sessions {
		events[i] = ical.ExportEvent{
			UID:      fmt.Sprintf("session-%d@team-scheduler", ts.ID),
			Summary:  "Тренировка: " + t.Name,
			Location: ts.RoomName,
			Start:    ts.StartsAt,
			End:      ts.EndsAt,
		}
	}
	var buf bytes.Buffer
	if err := ical.Write(&buf, t.Name, events, s.now()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
