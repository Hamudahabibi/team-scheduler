package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

// AvailabilityRepo — хранилище для сервиса доступности.
type AvailabilityRepo interface {
	availabilitySource
	ReplaceRules(ctx context.Context, userID int64, rules []domain.AvailabilityRule) error
	CreateException(ctx context.Context, e domain.AvailabilityException) (domain.AvailabilityException, error)
	DeleteException(ctx context.Context, userID, id int64) error
	TeamByID(ctx context.Context, id int64) (domain.Team, error)
	TeamMembers(ctx context.Context, teamID int64) ([]domain.TeamMember, error)
	UserByID(ctx context.Context, id int64) (domain.User, error)
}

// Availability — доступность игроков.
type Availability struct {
	repo   AvailabilityRepo
	loc    *time.Location
	buffer time.Duration
}

// NewAvailability создаёт сервис. buffer — запас времени до и после пары.
func NewAvailability(repo AvailabilityRepo, loc *time.Location, buffer time.Duration) *Availability {
	return &Availability{repo: repo, loc: loc, buffer: buffer}
}

// maxRulesPerUser ограничивает число правил: на неделю их нужно единицы.
const maxRulesPerUser = 50

// Rules возвращает еженедельные правила текущего пользователя.
func (s *Availability) Rules(ctx context.Context, actor domain.Actor) ([]domain.AvailabilityRule, error) {
	return s.repo.RulesByUsers(ctx, []int64{actor.UserID})
}

// SetRules заменяет еженедельные правила текущего пользователя.
// Пустой список означает «свободен всегда, кроме пар».
func (s *Availability) SetRules(ctx context.Context, actor domain.Actor, rules []domain.AvailabilityRule) ([]domain.AvailabilityRule, error) {
	if len(rules) > maxRulesPerUser {
		return nil, domain.Invalid("rules", fmt.Sprintf("не больше %d правил", maxRulesPerUser))
	}
	for i, r := range rules {
		field := fmt.Sprintf("rules[%d]", i)
		if err := domain.ValidateWeekMinutes(field, r.Weekday, r.StartMin, r.EndMin); err != nil {
			return nil, err
		}
		if r.Kind != domain.RuleAvailable && r.Kind != domain.RulePreferred {
			return nil, domain.Invalid(field, "kind: available или preferred")
		}
	}
	if err := s.repo.ReplaceRules(ctx, actor.UserID, rules); err != nil {
		return nil, err
	}
	return s.repo.RulesByUsers(ctx, []int64{actor.UserID})
}

// Exceptions возвращает исключения текущего пользователя за период.
func (s *Availability) Exceptions(ctx context.Context, actor domain.Actor, from, to time.Time) ([]domain.AvailabilityException, error) {
	if !to.After(from) {
		return nil, domain.Invalid("to", "конец периода должен быть позже начала")
	}
	return s.repo.ExceptionsByUsers(ctx, []int64{actor.UserID}, from, to)
}

// AddException добавляет исключение текущему пользователю.
func (s *Availability) AddException(ctx context.Context, actor domain.Actor, e domain.AvailabilityException) (domain.AvailabilityException, error) {
	if !e.EndsAt.After(e.StartsAt) {
		return domain.AvailabilityException{}, domain.Invalid("ends_at", "конец должен быть позже начала")
	}
	if e.EndsAt.Sub(e.StartsAt) > 120*24*time.Hour {
		return domain.AvailabilityException{}, domain.Invalid("ends_at", "исключение не длиннее 120 дней")
	}
	if e.Kind != domain.ExceptionAvailable && e.Kind != domain.ExceptionUnavailable {
		return domain.AvailabilityException{}, domain.Invalid("kind", "available или unavailable")
	}
	e.UserID = actor.UserID
	e.Note = strings.TrimSpace(e.Note)
	return s.repo.CreateException(ctx, e)
}

// DeleteException удаляет исключение текущего пользователя.
func (s *Availability) DeleteException(ctx context.Context, actor domain.Actor, id int64) error {
	return s.repo.DeleteException(ctx, actor.UserID, id)
}

// Interval — непрерывный интервал времени.
type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// WeekView — доступность игрока на неделю, как её видит планировщик.
type WeekView struct {
	WeekStart time.Time  `json:"week_start"`
	Free      []Interval `json:"free"`
	Preferred []Interval `json:"preferred"`
	Classes   []Interval `json:"classes"`
}

// MyWeek показывает игроку итоговую доступность на неделю: так проще
// проверить, что пары и правила учтены правильно.
func (s *Availability) MyWeek(ctx context.Context, actor domain.Actor, weekStart time.Time) (WeekView, error) {
	w, err := timegrid.NewWeek(weekStart, s.loc)
	if err != nil {
		return WeekView{}, domain.Invalid("week", err.Error())
	}
	u, err := s.repo.UserByID(ctx, actor.UserID)
	if err != nil {
		return WeekView{}, err
	}
	masks, err := loadPlayerMasks(ctx, s.repo, w, []playerRef{{UserID: u.ID, GroupID: u.StudyGroupID}}, s.buffer)
	if err != nil {
		return WeekView{}, err
	}
	m := masks[u.ID]
	return WeekView{
		WeekStart: w.Start(),
		Free:      maskIntervals(w, m.Free),
		Preferred: maskIntervals(w, m.Preferred),
		Classes:   maskIntervals(w, m.Classes),
	}, nil
}

// Heatmap — сколько игроков команды свободно в каждый слот недели.
// Индексы: [день 0..6, пн = 0][слот дня 0..47, 00:00 = 0].
type Heatmap struct {
	WeekStart    time.Time                                `json:"week_start"`
	CoreTotal    int                                      `json:"core_total"`
	MembersTotal int                                      `json:"members_total"`
	CoreFree     [timegrid.Days][timegrid.SlotsPerDay]int `json:"core_free"`
	AllFree      [timegrid.Days][timegrid.SlotsPerDay]int `json:"all_free"`
}

// TeamHeatmap строит тепловую карту доступности команды. Доступна
// администратору, тренеру команды и её игрокам.
func (s *Availability) TeamHeatmap(ctx context.Context, actor domain.Actor, teamID int64, weekStart time.Time) (Heatmap, error) {
	w, err := timegrid.NewWeek(weekStart, s.loc)
	if err != nil {
		return Heatmap{}, domain.Invalid("week", err.Error())
	}
	t, err := s.repo.TeamByID(ctx, teamID)
	if err != nil {
		return Heatmap{}, err
	}
	members, err := s.repo.TeamMembers(ctx, teamID)
	if err != nil {
		return Heatmap{}, err
	}
	isMember := slices.ContainsFunc(members, func(m domain.TeamMember) bool { return m.UserID == actor.UserID })
	if !canManageTeam(actor, t) && !isMember {
		return Heatmap{}, domain.ErrForbidden
	}

	refs := make([]playerRef, len(members))
	for i, m := range members {
		refs[i] = playerRef{UserID: m.UserID, GroupID: m.StudyGroupID}
	}
	masks, err := loadPlayerMasks(ctx, s.repo, w, refs, s.buffer)
	if err != nil {
		return Heatmap{}, err
	}

	h := Heatmap{WeekStart: w.Start(), MembersTotal: len(members)}
	for _, m := range members {
		core := m.Role == domain.MemberCore
		if core {
			h.CoreTotal++
		}
		free := masks[m.UserID].Free
		for slot := range timegrid.SlotsPerWeek {
			if !free.Has(slot) {
				continue
			}
			d, sd := timegrid.DayOf(slot), timegrid.SlotOfDay(slot)
			h.AllFree[d][sd]++
			if core {
				h.CoreFree[d][sd]++
			}
		}
	}
	return h, nil
}

// maskIntervals превращает маску в список непрерывных интервалов.
func maskIntervals(w timegrid.Week, m timegrid.Mask) []Interval {
	res := []Interval{}
	for slot := 0; slot < timegrid.SlotsPerWeek; {
		if !m.Has(slot) {
			slot++
			continue
		}
		start := slot
		for slot < timegrid.SlotsPerWeek && m.Has(slot) {
			slot++
		}
		res = append(res, Interval{Start: w.SlotTime(start), End: w.SlotTime(slot)})
	}
	return res
}
