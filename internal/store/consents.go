package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const PolicyLGPDExtractV1 = "lgpd-extract-v1"

type Consents struct {
	pool *pgxpool.Pool
}

func NewConsents(pool *pgxpool.Pool) *Consents {
	return &Consents{pool: pool}
}

type ConsentRecord struct {
	UserSub       string
	UserEmail     string
	IP            string
	UserAgent     string
	PolicyVersion string
	ContentSHA256 string
	DocType       string
	AcceptedAt    time.Time
}

func (s *Consents) Insert(ctx context.Context, rec ConsentRecord) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("consent store not configured")
	}
	if rec.AcceptedAt.IsZero() {
		rec.AcceptedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO extract_consents (
			user_sub, user_email, ip, user_agent, policy_version,
			content_sha256, doc_type, accepted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		rec.UserSub,
		nullIfEmpty(rec.UserEmail),
		nullIfEmpty(rec.IP),
		nullIfEmpty(rec.UserAgent),
		rec.PolicyVersion,
		nullIfEmpty(rec.ContentSHA256),
		nullIfEmpty(rec.DocType),
		rec.AcceptedAt,
	)
	if err != nil {
		return fmt.Errorf("insert extract consent: %w", err)
	}
	return nil
}
