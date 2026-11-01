package domain

import "time"

// ClassEvent — занятие учебной группы (одна пара в конкретный день).
type ClassEvent struct {
	ID       int64     `json:"id"`
	GroupID  int64     `json:"group_id"`
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}
