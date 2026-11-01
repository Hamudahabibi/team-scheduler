package domain

import "time"

// Room — компьютерный зал киберцентра.
type Room struct {
	ID    int64       `json:"id"`
	Name  string      `json:"name"`
	Seats int         `json:"seats"`
	Hours []RoomHours `json:"hours"`
}

// RoomHours — часы работы зала в один день недели (1 = понедельник).
type RoomHours struct {
	Weekday  int `json:"weekday"`
	OpenMin  int `json:"open_min"`
	CloseMin int `json:"close_min"`
}

// RoomBlock — интервал, когда зал недоступен для тренировок.
type RoomBlock struct {
	ID       int64     `json:"id"`
	RoomID   int64     `json:"room_id"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Reason   string    `json:"reason"`
}

// ValidateWeekMinutes проверяет интервал внутри дня недели:
// день 1..7, минуты 0..1440, конец позже начала.
func ValidateWeekMinutes(field string, weekday, startMin, endMin int) error {
	switch {
	case weekday < 1 || weekday > 7:
		return Invalid(field, "день недели от 1 (пн) до 7 (вс)")
	case startMin < 0 || endMin > 24*60:
		return Invalid(field, "время от 0 до 1440 минут")
	case endMin <= startMin:
		return Invalid(field, "конец интервала должен быть позже начала")
	}
	return nil
}
