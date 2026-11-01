package domain

import "time"

// AttendanceStatus — отметка о посещении тренировки.
type AttendanceStatus string

const (
	AttendancePresent AttendanceStatus = "present"
	AttendanceLate    AttendanceStatus = "late"
	AttendanceAbsent  AttendanceStatus = "absent"
	AttendanceExcused AttendanceStatus = "excused"
)

// Valid сообщает, известна ли отметка.
func (s AttendanceStatus) Valid() bool {
	switch s {
	case AttendancePresent, AttendanceLate, AttendanceAbsent, AttendanceExcused:
		return true
	}
	return false
}

// Attended — игрок был на тренировке (вовремя или с опозданием).
func (s AttendanceStatus) Attended() bool { return s == AttendancePresent || s == AttendanceLate }

// AttendanceMark — отметка игрока на тренировке.
type AttendanceMark struct {
	SessionID int64            `json:"session_id"`
	UserID    int64            `json:"user_id"`
	FullName  string           `json:"full_name,omitempty"`
	Status    AttendanceStatus `json:"status"`
	MarkedBy  *int64           `json:"marked_by"`
	MarkedAt  time.Time        `json:"marked_at"`
}

// SessionAttendance — посещаемость одной тренировки для отчёта.
type SessionAttendance struct {
	SessionID          int64
	StartsAt           time.Time
	ExpectedAttendance float64
	Marks              []AttendanceMark
}
