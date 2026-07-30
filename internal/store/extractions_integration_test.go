package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/db"
	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/store"
)

func TestExtractionsInsertReplaceIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	plain, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	_ = plain

	keyRec, err := store.NewAPIKeys(pool).Create(ctx, store.CreateAPIKeyInput{
		Name:      "integration",
		KeyPrefix: prefix,
		KeyHash:   hash,
		Scopes:    []string{auth.ScopeExtractWrite},
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	ex := store.NewExtractions(pool)
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	rec := store.ExtractionRecord{
		DocType:       "identity_document",
		ContentSHA256: sha,
		Filename:      "doc.png",
		MIME:          "image/png",
		Mode:          "vision",
		Model:         "test",
		RequestID:     "req-1",
		ClientIP:      "203.0.113.9",
		UserAgent:     "integration-test",
		APIKeyID:      keyRec.ID,
		AuthSubject:   "api_key:" + keyRec.ID,
		Status:        "success",
		Result: extract.Result{
			DocType: "identity_document",
			Data:    map[string]any{"nome": "TEST"},
		},
		Duration: 12 * time.Millisecond,
	}
	if err := ex.Insert(ctx, rec); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := ex.Replace(ctx, rec); err != nil {
		t.Fatalf("replace: %v", err)
	}

	var active int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM extractions
		WHERE content_sha256 = $1 AND deleted_at IS NULL
	`, sha).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active rows=%d want 1", active)
	}

	var clientIP, subject string
	if err := pool.QueryRow(ctx, `
		SELECT client_ip, auth_subject FROM extractions
		WHERE content_sha256 = $1 AND deleted_at IS NULL
	`, sha).Scan(&clientIP, &subject); err != nil {
		t.Fatal(err)
	}
	if clientIP != "203.0.113.9" || subject == "" {
		t.Fatalf("client_ip=%q subject=%q", clientIP, subject)
	}
}
