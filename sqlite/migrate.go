package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"log/slog"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	if logger != nil {
		goose.SetLogger(slog.NewLogLogger(logger.Handler(), slog.LevelInfo))
	}
	goose.SetBaseFS(migrationsFS)
	return goose.UpContext(ctx, db, "migrations")
}
