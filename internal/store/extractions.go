package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kalke/personal-document-extractor/internal/extract"
)

type Extractions struct {
	pool *pgxpool.Pool
}

func NewExtractions(pool *pgxpool.Pool) *Extractions {
	return &Extractions{pool: pool}
}

type ExtractionRecord struct {
	DocType       string
	ContentSHA256 string
	Filename      string
	MIME          string
	Mode          string
	Model         string
	RequestID     string
	ClientIP      string
	UserAgent     string
	Status        string
	Result        extract.Result
	Duration      time.Duration
}

func (s *Extractions) Insert(ctx context.Context, rec ExtractionRecord) error {
	return s.persist(ctx, rec, false)
}

func (s *Extractions) Replace(ctx context.Context, rec ExtractionRecord) error {
	return s.persist(ctx, rec, true)
}

func (s *Extractions) persist(ctx context.Context, rec ExtractionRecord, replace bool) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("store not configured")
	}
	payload, err := json.Marshal(rec.Result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if replace {
		if _, err := tx.Exec(ctx, `
			UPDATE extractions
			SET deleted_at = now()
			WHERE doc_type = $1
			  AND content_sha256 = $2
			  AND deleted_at IS NULL
		`, rec.DocType, rec.ContentSHA256); err != nil {
			return fmt.Errorf("soft delete extraction: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO extractions (
			doc_type, content_sha256, filename, mime, mode, model,
			request_id, client_ip, user_agent, status, result_json, duration_ms
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		rec.DocType,
		rec.ContentSHA256,
		rec.Filename,
		rec.MIME,
		rec.Mode,
		rec.Model,
		rec.RequestID,
		rec.ClientIP,
		rec.UserAgent,
		rec.Status,
		payload,
		int(rec.Duration.Milliseconds()),
	); err != nil {
		return fmt.Errorf("insert extraction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit extraction: %w", err)
	}
	return nil
}
