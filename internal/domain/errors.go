// Package domain содержит сущности предметной области и доменные ошибки.
package domain

import (
	"errors"
	"fmt"
)

// Базовые ошибки. Сервисы оборачивают их с подробностями,
// HTTP-слой по ним выбирает код ответа через errors.Is.
var (
	ErrNotFound     = errors.New("не найдено")
	ErrConflict     = errors.New("конфликт")
	ErrForbidden    = errors.New("недостаточно прав")
	ErrUnauthorized = errors.New("требуется вход")
	ErrValidation   = errors.New("некорректные данные")
)

// ValidationError описывает ошибку в конкретном поле запроса.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Unwrap позволяет проверять ошибку через errors.Is(err, ErrValidation).
func (e *ValidationError) Unwrap() error { return ErrValidation }

// Invalid — короткий конструктор ValidationError.
func Invalid(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
