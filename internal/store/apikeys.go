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
	UserID    string
	Name      string
	KeyPrefix string
	KeyHash   string
	Scopes    []string
	ExpiresAt *time.Time
}

type APIKeyPublic struct {
	ID        string
	UserID    string
	Name      string
	KeyPrefix string
	Scopes    []string
	ExpiresAt *time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (s *APIKeys) Create(ctx context.Context, in CreateAPIKeyInput) (auth.APIKeyRecord, error) {
	if s == nil || s.pool == nil {
		return auth.APIKeyRecord{}, fmt.Errorf("store not configured")
	}
	if in.UserID == "" {
		return auth.APIKeyRecord{}, fmt.Errorf("user_id is required")
	}
	var rec auth.APIKeyRecord
	err := s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (user_id, name, key_prefix, key_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, user_id::text, name, key_prefix, key_hash, scopes, expires_at, revoked_at, created_at
	`, in.UserID, in.Name, in.KeyPrefix, in.KeyHash, in.Scopes, in.ExpiresAt).Scan(
		&rec.ID,
		&rec.UserID,
		&rec.Name,
		&rec.KeyPrefix,
		&rec.KeyHash,
		&rec.Scopes,
		&rec.ExpiresAt,
		&rec.RevokedAt,
		&rec.CreatedAt,
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
		SELECT id::text, user_id::text, name, key_prefix, key_hash, scopes, expires_at, revoked_at
		FROM api_keys
		WHERE key_prefix = $1
		LIMIT 1
	`, prefix).Scan(
		&rec.ID,
		&rec.UserID,
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

func (s *APIKeys) ListByUser(ctx context.Context, userID string) ([]APIKeyPublic, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("store not configured")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, name, key_prefix, scopes, expires_at, revoked_at, created_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var out []APIKeyPublic
	for rows.Next() {
		var k APIKeyPublic
		if err := rows.Scan(
			&k.ID,
			&k.UserID,
			&k.Name,
			&k.KeyPrefix,
			&k.Scopes,
			&k.ExpiresAt,
			&k.RevokedAt,
			&k.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

var ErrAPIKeyNotFound = errors.New("api key not found")

func (s *APIKeys) RevokeForUser(ctx context.Context, userID, keyID string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("store not configured")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE api_keys
		SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, keyID, userID)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}
