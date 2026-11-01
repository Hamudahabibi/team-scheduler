package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/ical"
)

// AcademicRepo — хранилище учебных групп и занятий.
type AcademicRepo interface {
	ListGroups(ctx context.Context) ([]domain.StudyGroup, error)
	CreateGroup(ctx context.Context, code string) (domain.StudyGroup, error)
	GroupByID(ctx context.Context, id int64) (domain.StudyGroup, error)
	ReplaceClasses(ctx context.Context, groupID int64, from, to time.Time, classes []domain.ClassEvent) error
	ClassesByGroups(ctx context.Context, groupIDs []int64, from, to time.Time) ([]domain.ClassEvent, error)
}

// Academic — учебные группы и их расписание.
type Academic struct {
	repo AcademicRepo
	loc  *time.Location
}

// NewAcademic создаёт сервис. loc — часовой пояс для времени без смещения.
func NewAcademic(repo AcademicRepo, loc *time.Location) *Academic {
	return &Academic{repo: repo, loc: loc}
}

// ScheduleFormat — формат загружаемого расписания.
type ScheduleFormat string

const (
	FormatICS  ScheduleFormat = "ics"
	FormatJSON ScheduleFormat = "json"
)

// maxImportPeriod — импорт дольше учебного года почти наверняка ошибка.
const maxImportPeriod = 366 * 24 * time.Hour

// Groups возвращает учебные группы.
func (s *Academic) Groups(ctx context.Context) ([]domain.StudyGroup, error) {
	return s.repo.ListGroups(ctx)
}

// CreateGroup создаёт учебную группу. Только администратор.
func (s *Academic) CreateGroup(ctx context.Context, actor domain.Actor, code string) (domain.StudyGroup, error) {
	if !actor.IsAdmin() {
		return domain.StudyGroup{}, domain.ErrForbidden
	}
	code = normalizeGroupCode(code)
	if code == "" {
		return domain.StudyGroup{}, domain.Invalid("code", "укажите код группы")
	}
	return s.repo.CreateGroup(ctx, code)
}

// jsonClass — формат занятия при импорте из JSON.
type jsonClass struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// ImportSchedule заменяет занятия группы, начинающиеся в [from, to),
// данными из файла. Возвращает число загруженных занятий.
func (s *Academic) ImportSchedule(ctx context.Context, actor domain.Actor, groupID int64,
	format ScheduleFormat, data []byte, from, to time.Time,
) (int, error) {
	if !actor.IsAdmin() {
		return 0, domain.ErrForbidden
	}
	if !to.After(from) {
		return 0, domain.Invalid("to", "конец периода должен быть позже начала")
	}
	if to.Sub(from) > maxImportPeriod {
		return 0, domain.Invalid("to", "период импорта не больше года")
	}
	if _, err := s.repo.GroupByID(ctx, groupID); err != nil {
		return 0, err
	}

	var classes []domain.ClassEvent
	switch format {
	case FormatICS:
		events, err := ical.Parse(bytes.NewReader(data), s.loc)
		if err != nil {
			return 0, domain.Invalid("body", fmt.Sprintf("разбор iCalendar: %v", err))
		}
		for _, o := range ical.Expand(events, from, to) {
			classes = append(classes, domain.ClassEvent{Title: classTitle(o), StartsAt: o.Start, EndsAt: o.End})
		}
	case FormatJSON:
		var items []jsonClass
		if err := json.Unmarshal(data, &items); err != nil {
			return 0, domain.Invalid("body", fmt.Sprintf("разбор JSON: %v", err))
		}
		for i, it := range items {
			if !it.EndsAt.After(it.StartsAt) {
				return 0, domain.Invalid(fmt.Sprintf("body[%d]", i), "конец занятия должен быть позже начала")
			}
			classes = append(classes, domain.ClassEvent{Title: strings.TrimSpace(it.Title), StartsAt: it.StartsAt, EndsAt: it.EndsAt})
		}
	default:
		return 0, domain.Invalid("format", "поддерживаются ics и json")
	}

	// Учитываются только занятия, начинающиеся в периоде: так же их удаляет ReplaceClasses.
	inPeriod := classes[:0]
	for _, c := range classes {
		if !c.StartsAt.Before(from) && c.StartsAt.Before(to) {
			inPeriod = append(inPeriod, c)
		}
	}
	if err := s.repo.ReplaceClasses(ctx, groupID, from, to, inPeriod); err != nil {
		return 0, err
	}
	return len(inPeriod), nil
}

func classTitle(o ical.Occurrence) string {
	if o.Location == "" {
		return o.Summary
	}
	return o.Summary + " (" + o.Location + ")"
}

// Classes возвращает занятия группы за период.
func (s *Academic) Classes(ctx context.Context, groupID int64, from, to time.Time) ([]domain.ClassEvent, error) {
	if !to.After(from) {
		return nil, domain.Invalid("to", "конец периода должен быть позже начала")
	}
	return s.repo.ClassesByGroups(ctx, []int64{groupID}, from, to)
}
