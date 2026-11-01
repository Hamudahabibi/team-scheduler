package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Hamudahabibi/team-scheduler/migrations"
)

// Migrate применяет все новые миграции. Вызывается при старте сервиса.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	// Закрытие *sql.DB не закрывает исходный пул.
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("инициализация миграций: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("применение миграций: %w", err)
	}
	for _, r := range results {
		log.Info("миграция применена",
			slog.String("file", r.Source.Path), slog.Duration("duration", r.Duration))
	}
	return nil
}
