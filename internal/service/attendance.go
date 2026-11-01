package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// AttendanceRepo — хранилище для сервиса посещаемости.
type AttendanceRepo interface {
	SessionByID(ctx context.Context, id int64) (domain.TrainingSession, error)
	TeamByID(ctx context.Context, id int64) (domain.Team, error)
	TeamMembers(ctx context.Context, teamID int64) ([]domain.TeamMember, error)
	UpsertAttendance(ctx context.Context, marks []domain.AttendanceMark) error
	AttendanceBySession(ctx context.Context, sessionID int64) ([]domain.AttendanceMark, error)
	TeamAttendance(ctx context.Context, teamID int64, from, to time.Time) ([]domain.SessionAttendance, error)
}

// Attendance — отметки посещаемости и статистика.
type Attendance struct {
	repo AttendanceRepo
	now  func() time.Time
	// substituteWeight — вес запасного, как в целевой функции планировщика,
	// чтобы фактическая посещаемость сравнивалась с прогнозом в одной шкале.
	substituteWeight float64
}

// NewAttendance создаёт сервис посещаемости.
func NewAttendance(repo AttendanceRepo, substituteWeight float64) *Attendance {
	return &Attendance{repo: repo, now: time.Now, substituteWeight: substituteWeight}
}

// MarkInput — отметка одного игрока.
type MarkInput struct {
	UserID int64
	Status domain.AttendanceStatus
}

// Mark сохраняет отметки тренировки. Тренер команды или администратор,
// только для опубликованной тренировки, которая уже началась.
func (s *Attendance) Mark(ctx context.Context, actor domain.Actor, sessionID int64, marks []MarkInput) ([]domain.AttendanceMark, error) {
	ts, err := s.repo.SessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.TeamByID(ctx, ts.TeamID)
	if err != nil {
		return nil, err
	}
	if !canManageTeam(actor, t) {
		return nil, domain.ErrForbidden
	}
	if ts.Status != domain.SessionPublished {
		return nil, fmt.Errorf("отмечать можно только опубликованную тренировку: %w", domain.ErrConflict)
	}
	if s.now().Before(ts.StartsAt) {
		return nil, fmt.Errorf("тренировка ещё не началась: %w", domain.ErrConflict)
	}
	members, err := s.repo.TeamMembers(ctx, ts.TeamID)
	if err != nil {
		return nil, err
	}

	res := make([]domain.AttendanceMark, 0, len(marks))
	for i, m := range marks {
		field := fmt.Sprintf("marks[%d]", i)
		if !m.Status.Valid() {
			return nil, domain.Invalid(field, "status: present, late, absent или excused")
		}
		if !slices.ContainsFunc(members, func(tm domain.TeamMember) bool { return tm.UserID == m.UserID }) {
			return nil, domain.Invalid(field, fmt.Sprintf("игрок %d не состоит в команде", m.UserID))
		}
		res = append(res, domain.AttendanceMark{SessionID: sessionID, UserID: m.UserID, Status: m.Status, MarkedBy: &actor.UserID})
	}
	if err := s.repo.UpsertAttendance(ctx, res); err != nil {
		return nil, err
	}
	return s.repo.AttendanceBySession(ctx, sessionID)
}

// SessionMarks возвращает отметки тренировки. Доступно тренеру и игрокам команды.
func (s *Attendance) SessionMarks(ctx context.Context, actor domain.Actor, sessionID int64) ([]domain.AttendanceMark, error) {
	ts, err := s.repo.SessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if _, err := s.teamForViewer(ctx, actor, ts.TeamID); err != nil {
		return nil, err
	}
	return s.repo.AttendanceBySession(ctx, sessionID)
}

func (s *Attendance) teamForViewer(ctx context.Context, actor domain.Actor, teamID int64) ([]domain.TeamMember, error) {
	t, err := s.repo.TeamByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.TeamMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	isMember := slices.ContainsFunc(members, func(m domain.TeamMember) bool { return m.UserID == actor.UserID })
	if !canManageTeam(actor, t) && !isMember {
		return nil, domain.ErrForbidden
	}
	return members, nil
}

// PlayerStats — посещаемость игрока за период.
type PlayerStats struct {
	UserID   int64             `json:"user_id"`
	FullName string            `json:"full_name"`
	Role     domain.MemberRole `json:"role"`
	Marked   int               `json:"marked"`
	Present  int               `json:"present"`
	Late     int               `json:"late"`
	Absent   int               `json:"absent"`
	Excused  int               `json:"excused"`
	// Rate — доля посещённых (вовремя или с опозданием) среди отмеченных.
	Rate float64 `json:"rate"`
}

// TeamStats — посещаемость команды за период.
type TeamStats struct {
	TeamID int64     `json:"team_id"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	// Sessions — опубликованных тренировок в периоде, Marked — из них с отметками.
	Sessions int `json:"sessions"`
	Marked   int `json:"marked"`
	// AvgExpected — средний прогноз посещаемости планировщика по отмеченным тренировкам,
	// AvgActual — средняя фактическая посещаемость в той же шкале (запасной с весом 0,5).
	AvgExpected float64       `json:"avg_expected"`
	AvgActual   float64       `json:"avg_actual"`
	Players     []PlayerStats `json:"players"`
}

// TeamStats считает посещаемость команды. Учитываются игроки текущего состава.
func (s *Attendance) TeamStats(ctx context.Context, actor domain.Actor, teamID int64, from, to time.Time) (TeamStats, error) {
	if !to.After(from) {
		return TeamStats{}, domain.Invalid("to", "конец периода должен быть позже начала")
	}
	members, err := s.teamForViewer(ctx, actor, teamID)
	if err != nil {
		return TeamStats{}, err
	}
	sessions, err := s.repo.TeamAttendance(ctx, teamID, from, to)
	if err != nil {
		return TeamStats{}, err
	}

	res := TeamStats{TeamID: teamID, From: from, To: to, Sessions: len(sessions), Players: []PlayerStats{}}
	weights := map[int64]float64{}
	total := 0.0
	byUser := map[int64]*PlayerStats{}
	for _, m := range members {
		w := 1.0
		if m.Role != domain.MemberCore {
			w = s.substituteWeight
		}
		weights[m.UserID] = w
		total += w
		res.Players = append(res.Players, PlayerStats{UserID: m.UserID, FullName: m.FullName, Role: m.Role})
	}
	for i := range res.Players {
		byUser[res.Players[i].UserID] = &res.Players[i]
	}

	for _, sa := range sessions {
		if len(sa.Marks) == 0 {
			continue
		}
		res.Marked++
		res.AvgExpected += sa.ExpectedAttendance
		attended := 0.0
		for _, m := range sa.Marks {
			ps := byUser[m.UserID]
			if ps == nil {
				continue // игрок уже не в составе
			}
			ps.Marked++
			switch m.Status {
			case domain.AttendancePresent:
				ps.Present++
			case domain.AttendanceLate:
				ps.Late++
			case domain.AttendanceAbsent:
				ps.Absent++
			case domain.AttendanceExcused:
				ps.Excused++
			}
			if m.Status.Attended() {
				attended += weights[m.UserID]
			}
		}
		if total > 0 {
			res.AvgActual += attended / total
		}
	}
	if res.Marked > 0 {
		res.AvgExpected /= float64(res.Marked)
		res.AvgActual /= float64(res.Marked)
	}
	for i := range res.Players {
		p := &res.Players[i]
		if p.Marked > 0 {
			p.Rate = float64(p.Present+p.Late) / float64(p.Marked)
		}
	}
	return res, nil
}
