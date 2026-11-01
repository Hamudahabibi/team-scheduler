// Package config читает конфигурацию сервиса из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config — настройки сервиса.
type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	LogLevel        string
	Timezone        *time.Location
	TokenTTL        time.Duration
	ShutdownTimeout time.Duration
	// ClassBufferMin — сколько минут до и после пары игрок считается занятым.
	ClassBufferMin int
	// SolverTimeLimit — лимит времени метода ветвей и границ.
	SolverTimeLimit time.Duration
	// AdminEmail — пользователь с этим e-mail при регистрации получает роль admin.
	AdminEmail string
}

// Load читает конфигурацию из окружения и проверяет её.
func Load() (Config, error) {
	var errs []error

	cfg := Config{
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		AdminEmail:  os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL не задан"))
	}

	tzName := getEnv("TIMEZONE", "Europe/Moscow")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		errs = append(errs, fmt.Errorf("TIMEZONE: %w", err))
	}
	cfg.Timezone = loc

	cfg.TokenTTL, err = getDuration("TOKEN_TTL", 7*24*time.Hour)
	errs = append(errs, err)
	cfg.ShutdownTimeout, err = getDuration("SHUTDOWN_TIMEOUT", 10*time.Second)
	errs = append(errs, err)
	cfg.SolverTimeLimit, err = getDuration("SOLVER_TIME_LIMIT", 3*time.Second)
	errs = append(errs, err)
	cfg.ClassBufferMin, err = getInt("CLASS_BUFFER_MIN", 10)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("конфигурация: %w", err)
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func getInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}
