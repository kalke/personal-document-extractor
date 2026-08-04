package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kalke/personal-document-extractor/internal/extract"
)

var ErrNotFound = errors.New("not found")

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
	AuthSubject   string
	AuthClient    string
	AuthEmail     string
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

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
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
			request_id, client_ip, user_agent, auth_subject, auth_client, auth_email,
			status, result_json, duration_ms
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
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
		nullIfEmpty(rec.AuthSubject),
		nullIfEmpty(rec.AuthClient),
		nullIfEmpty(rec.AuthEmail),
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

// ExtractionSummary is a stored extract returned to the owning user.
type ExtractionSummary struct {
	ID            string          `json:"id"`
	CreatedAt     time.Time       `json:"created_at"`
	DocType       string          `json:"doc_type"`
	Filename      string          `json:"filename,omitempty"`
	ContentSHA256 string          `json:"content_sha256"`
	Status        string          `json:"status"`
	Result        json.RawMessage `json:"result"`
}

func (s *Extractions) ListBySubject(ctx context.Context, subject, docType string, limit int) ([]ExtractionSummary, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("store not configured")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("subject required")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, created_at, doc_type, COALESCE(filename, ''), content_sha256, status, result_json
		FROM extractions
		WHERE auth_subject = $1
		  AND deleted_at IS NULL
		  AND ($2 = '' OR doc_type = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`, subject, docType, limit)
	if err != nil {
		return nil, fmt.Errorf("list extractions: %w", err)
	}
	defer rows.Close()

	out := make([]ExtractionSummary, 0)
	for rows.Next() {
		var item ExtractionSummary
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.DocType,
			&item.Filename,
			&item.ContentSHA256,
			&item.Status,
			&item.Result,
		); err != nil {
			return nil, fmt.Errorf("scan extraction: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Extractions) GetByIDForSubject(ctx context.Context, id, subject string) (ExtractionSummary, error) {
	if s == nil || s.pool == nil {
		return ExtractionSummary{}, fmt.Errorf("store not configured")
	}
	id = strings.TrimSpace(id)
	subject = strings.TrimSpace(subject)
	if id == "" || subject == "" {
		return ExtractionSummary{}, fmt.Errorf("id and subject required")
	}
	var item ExtractionSummary
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, created_at, doc_type, COALESCE(filename, ''), content_sha256, status, result_json
		FROM extractions
		WHERE id = $1::uuid
		  AND auth_subject = $2
		  AND deleted_at IS NULL
	`, id, subject).Scan(
		&item.ID,
		&item.CreatedAt,
		&item.DocType,
		&item.Filename,
		&item.ContentSHA256,
		&item.Status,
		&item.Result,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ExtractionSummary{}, ErrNotFound
		}
		return ExtractionSummary{}, fmt.Errorf("get extraction: %w", err)
	}
	return item, nil
}
