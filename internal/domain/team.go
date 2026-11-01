package domain

import "time"

// Discipline — киберспортивная дисциплина.
type Discipline struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	RosterSize int    `json:"roster_size"`
}

// Team — команда киберцентра.
type Team struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	DisciplineID int64         `json:"discipline_id"`
	CoachID      *int64        `json:"coach_id"`
	CreatedAt    time.Time     `json:"created_at"`
	Members      []TeamMember  `json:"members,omitempty"`
	Plan         *TrainingPlan `json:"plan,omitempty"`
}

// MemberRole — место игрока в составе.
type MemberRole string

const (
	MemberCore       MemberRole = "core"
	MemberSubstitute MemberRole = "substitute"
)

// Valid сообщает, известна ли роль в составе.
func (r MemberRole) Valid() bool { return r == MemberCore || r == MemberSubstitute }

// TeamMember — игрок в составе команды.
type TeamMember struct {
	TeamID       int64      `json:"team_id"`
	UserID       int64      `json:"user_id"`
	FullName     string     `json:"full_name"`
	StudyGroupID *int64     `json:"study_group_id"`
	Role         MemberRole `json:"role"`
	Position     string     `json:"position"`
	JoinedAt     time.Time  `json:"joined_at"`
}

// TrainingPlan — требования тренера к недельному микроциклу команды.
// Время внутри дня задаётся в минутах от полуночи.
type TrainingPlan struct {
	TeamID          int64     `json:"team_id"`
	SessionsPerWeek int       `json:"sessions_per_week"`
	DurationMin     int       `json:"duration_min"`
	MinCorePlayers  int       `json:"min_core_players"`
	DayStartMin     int       `json:"day_start_min"`
	DayEndMin       int       `json:"day_end_min"`
	MinRestMin      int       `json:"min_rest_min"`
	Priority        int       `json:"priority"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SlotMinutes — шаг сетки планирования. Длительности и границы окон
// должны быть кратны ему.
const SlotMinutes = 30

// Validate проверяет план до сохранения.
func (p TrainingPlan) Validate() error {
	switch {
	case p.SessionsPerWeek < 1 || p.SessionsPerWeek > 7:
		return Invalid("sessions_per_week", "от 1 до 7 тренировок в неделю")
	case p.DurationMin < SlotMinutes || p.DurationMin > 8*60 || p.DurationMin%SlotMinutes != 0:
		return Invalid("duration_min", "от 30 до 480 минут, кратно 30")
	case p.MinCorePlayers < 1:
		return Invalid("min_core_players", "нужен хотя бы один игрок основного состава")
	case p.DayStartMin < 0 || p.DayStartMin%SlotMinutes != 0:
		return Invalid("day_start_min", "от 0, кратно 30")
	case p.DayEndMin > 24*60 || p.DayEndMin%SlotMinutes != 0:
		return Invalid("day_end_min", "до 1440, кратно 30")
	case p.DayEndMin-p.DayStartMin < p.DurationMin:
		return Invalid("day_end_min", "окно дня короче тренировки")
	case p.MinRestMin < 0:
		return Invalid("min_rest_min", "не может быть отрицательным")
	case p.Priority < 1 || p.Priority > 10:
		return Invalid("priority", "от 1 до 10")
	}
	return nil
}
