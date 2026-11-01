package domain

import (
	"encoding/json"
	"time"
)

// RunStatus — статус запуска генерации.
type RunStatus string

const (
	RunDraft     RunStatus = "draft"
	RunPublished RunStatus = "published"
	RunDiscarded RunStatus = "discarded"
)

// ScheduleRun — результат одного запуска генерации расписания на неделю.
type ScheduleRun struct {
	ID          int64             `json:"id"`
	WeekStart   time.Time         `json:"week_start"`
	Algorithm   string            `json:"algorithm"`
	Status      RunStatus         `json:"status"`
	Objective   float64           `json:"objective"`
	Stats       json.RawMessage   `json:"stats"`
	TeamIDs     []int64           `json:"team_ids"`
	CreatedBy   *int64            `json:"created_by"`
	CreatedAt   time.Time         `json:"created_at"`
	PublishedAt *time.Time        `json:"published_at"`
	Sessions    []TrainingSession `json:"sessions,omitempty"`
}

// SessionStatus — статус тренировки.
type SessionStatus string

const (
	SessionDraft     SessionStatus = "draft"
	SessionPublished SessionStatus = "published"
	SessionCancelled SessionStatus = "cancelled"
)

// TrainingSession — тренировка команды в зале.
type TrainingSession struct {
	ID                 int64         `json:"id"`
	RunID              *int64        `json:"run_id"`
	TeamID             int64         `json:"team_id"`
	TeamName           string        `json:"team_name,omitempty"`
	RoomID             int64         `json:"room_id"`
	RoomName           string        `json:"room_name,omitempty"`
	StartsAt           time.Time     `json:"starts_at"`
	EndsAt             time.Time     `json:"ends_at"`
	Seats              int           `json:"seats"`
	Status             SessionStatus `json:"status"`
	Quality            float64       `json:"quality"`
	ExpectedAttendance float64       `json:"expected_attendance"`
}

// SessionFilter — условия выборки тренировок, пересекающих [From, To).
type SessionFilter struct {
	From, To time.Time
	Status   SessionStatus
	TeamIDs  []int64 // пусто — все команды
	RoomIDs  []int64 // пусто — все залы
}
