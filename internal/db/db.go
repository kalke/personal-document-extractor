package db

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxAttempts   = 30
	retryDelay    = time.Second
	pingTimeout   = 2 * time.Second
	DefaultSchema = "pde"
)

var schemaNameRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// NormalizeSearchPath returns a safe schema name (default pde).
func NormalizeSearchPath(path string) (string, error) {
	if path == "" {
		path = DefaultSchema
	}
	if !schemaNameRe.MatchString(path) {
		return "", fmt.Errorf("invalid DB_SEARCH_PATH %q", path)
	}
	return path, nil
}

func Connect(ctx context.Context, databaseURL string, searchPath ...string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1

	raw := DefaultSchema
	if len(searchPath) > 0 {
		raw = searchPath[0]
	}
	path, err := NormalizeSearchPath(raw)
	if err != nil {
		return nil, err
	}

	// Neon pooler (transaction mode) rejects search_path startup params and drops
	// session SET between transactions. Re-apply on every checkout.
	quoted := pgx.Identifier{path}.Sanitize()
	setSQL := "SET search_path TO " + quoted + ", public"
	cfg.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		if _, err := conn.Exec(ctx, setSQL); err != nil {
			slog.Error("set search_path failed", "err", err, "schema", path)
			return false, err
		}
		return true, nil
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, setSQL)
		return err
	}

	var pool *pgxpool.Pool
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, lastErr = pgxpool.NewWithConfig(ctx, cfg)
		if lastErr == nil {
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			lastErr = pool.Ping(pingCtx)
			cancel()
			if lastErr == nil {
				if err := EnsureTables(ctx, pool); err != nil {
					pool.Close()
					return nil, err
				}
				return pool, nil
			}
			pool.Close()
		}
		slog.Warn("database connect failed; retrying", "attempt", attempt, "err", lastErr)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
		}
	}
	return nil, fmt.Errorf("connect database: %w", lastErr)
}

// EnsureTables fails fast when migrations/search_path are wrong.
func EnsureTables(ctx context.Context, pool *pgxpool.Pool) error {
	checkCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	var extractions, consents *string
	if err := pool.QueryRow(checkCtx, `SELECT to_regclass('extractions')::text, to_regclass('extract_consents')::text`).
		Scan(&extractions, &consents); err != nil {
		return fmt.Errorf("check tables: %w", err)
	}
	if extractions == nil || consents == nil {
		return fmt.Errorf("required tables missing in search_path (extractions=%v extract_consents=%v); check DB_SEARCH_PATH and migrations", extractions, consents)
	}
	return nil
}
