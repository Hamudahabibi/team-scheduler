package domain

import "time"

// Role — роль пользователя в системе.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleCoach  Role = "coach"
	RolePlayer Role = "player"
)

// Valid сообщает, известна ли роль.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleCoach, RolePlayer:
		return true
	}
	return false
}

// User — пользователь системы.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	FullName     string    `json:"full_name"`
	Role         Role      `json:"role"`
	StudyGroupID *int64    `json:"study_group_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// Actor — тот, кто выполняет действие. Передаётся в сервисы для проверки прав.
type Actor struct {
	UserID int64
	Role   Role
}

// IsAdmin — сокращение для частой проверки.
func (a Actor) IsAdmin() bool { return a.Role == RoleAdmin }

// StudyGroup — учебная группа, например «ИДБ-22-04».
type StudyGroup struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
}
