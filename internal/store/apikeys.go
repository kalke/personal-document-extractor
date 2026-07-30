package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kalke/personal-document-extractor/internal/auth"
)

type APIKeys struct {
	pool *pgxpool.Pool
}

func NewAPIKeys(pool *pgxpool.Pool) *APIKeys {
	return &APIKeys{pool: pool}
}

type CreateAPIKeyInput struct {
	Name      string
	KeyPrefix string
	KeyHash   string
	Scopes    []string
	ExpiresAt *time.Time
}

func (s *APIKeys) Create(ctx context.Context, in CreateAPIKeyInput) (auth.APIKeyRecord, error) {
	if s == nil || s.pool == nil {
		return auth.APIKeyRecord{}, fmt.Errorf("store not configured")
	}
	var rec auth.APIKeyRecord
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (name, key_prefix, key_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, name, key_prefix, key_hash, scopes, expires_at, revoked_at, created_at
	`, in.Name, in.KeyPrefix, in.KeyHash, in.Scopes, in.ExpiresAt).Scan(
		&rec.ID,
		&rec.Name,
		&rec.KeyPrefix,
		&rec.KeyHash,
		&rec.Scopes,
		&rec.ExpiresAt,
		&rec.RevokedAt,
		&createdAt,
	)
	if err != nil {
		return auth.APIKeyRecord{}, fmt.Errorf("insert api key: %w", err)
	}
	return rec, nil
}

func (s *APIKeys) LookupByPrefix(ctx context.Context, prefix string) (auth.APIKeyRecord, error) {
	if s == nil || s.pool == nil {
		return auth.APIKeyRecord{}, fmt.Errorf("store not configured")
	}
	var rec auth.APIKeyRecord
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, key_prefix, key_hash, scopes, expires_at, revoked_at
		FROM api_keys
		WHERE key_prefix = $1
		LIMIT 1
	`, prefix).Scan(
		&rec.ID,
		&rec.Name,
		&rec.KeyPrefix,
		&rec.KeyHash,
		&rec.Scopes,
		&rec.ExpiresAt,
		&rec.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.APIKeyRecord{}, auth.ErrUnauthorized
	}
	if err != nil {
		return auth.APIKeyRecord{}, fmt.Errorf("lookup api key: %w", err)
	}
	return rec, nil
}
