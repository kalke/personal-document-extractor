package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/kalke/personal-document-extractor/internal/db"
	"github.com/kalke/personal-document-extractor/migrations"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	maxAttempts = 30
	retryDelay  = time.Second
	pingTimeout = 2 * time.Second
)

func Up(ctx context.Context, databaseURL string, searchPath ...string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(1)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		lastErr = sqlDB.PingContext(pingCtx)
		cancel()
		if lastErr == nil {
			break
		}
		slog.Warn("migrate ping failed; retrying", "attempt", attempt, "err", lastErr)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("ping db: %w", lastErr)
	}

	raw := db.DefaultSchema
	if len(searchPath) > 0 && searchPath[0] != "" {
		raw = searchPath[0]
	}
	norm, err := db.NormalizeSearchPath(raw)
	if err != nil {
		return err
	}
	ident := `"` + norm + `"`
	if _, err := sqlDB.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+ident); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, "SET search_path TO "+ident+", public"); err != nil {
		return fmt.Errorf("search_path: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
