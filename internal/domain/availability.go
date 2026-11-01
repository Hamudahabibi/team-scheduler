package domain

import "time"

// RuleKind — вид еженедельного правила доступности.
type RuleKind string

const (
	// RuleAvailable — «могу» в это время.
	RuleAvailable RuleKind = "available"
	// RulePreferred — «могу и предпочитаю»: тоже доступность, но с бонусом в целевой функции.
	RulePreferred RuleKind = "preferred"
)

// AvailabilityRule — еженедельный интервал доступности игрока.
type AvailabilityRule struct {
	ID       int64    `json:"id"`
	UserID   int64    `json:"user_id"`
	Weekday  int      `json:"weekday"`
	StartMin int      `json:"start_min"`
	EndMin   int      `json:"end_min"`
	Kind     RuleKind `json:"kind"`
}

// ExceptionKind — вид исключения на конкретные даты.
type ExceptionKind string

const (
	ExceptionUnavailable ExceptionKind = "unavailable"
	ExceptionAvailable   ExceptionKind = "available"
)

// AvailabilityException — разовое изменение доступности (болезнь, сессия, поездка).
type AvailabilityException struct {
	ID       int64         `json:"id"`
	UserID   int64         `json:"user_id"`
	StartsAt time.Time     `json:"starts_at"`
	EndsAt   time.Time     `json:"ends_at"`
	Kind     ExceptionKind `json:"kind"`
	Note     string        `json:"note"`
}
