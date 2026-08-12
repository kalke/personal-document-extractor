package store_test

import (
	"context"
	"os"
	"testing"
	"time"

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
		AuthSubject:   "oidc|extraction-integration",
		AuthClient:    "pde-m2m",
		AuthEmail:     "",
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
	other := rec
	other.AuthSubject = "oidc|other-user"
	if err := ex.Insert(ctx, other); err != nil {
		t.Fatalf("insert other: %v", err)
	}
	if err := ex.Replace(ctx, rec); err != nil {
		t.Fatalf("replace: %v", err)
	}

	var otherActive int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM extractions
		WHERE content_sha256 = $1 AND auth_subject = $2 AND deleted_at IS NULL
	`, sha, other.AuthSubject).Scan(&otherActive); err != nil {
		t.Fatal(err)
	}
	if otherActive != 1 {
		t.Fatalf("other subject active=%d want 1", otherActive)
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

	var clientIP, subject, client, email string
	if err := pool.QueryRow(ctx, `
		SELECT client_ip, auth_subject, COALESCE(auth_client, ''), COALESCE(auth_email, '')
		FROM extractions
		WHERE content_sha256 = $1 AND deleted_at IS NULL
	`, sha).Scan(&clientIP, &subject, &client, &email); err != nil {
		t.Fatal(err)
	}
	if clientIP != "203.0.113.9" || subject == "" || client != "pde-m2m" {
		t.Fatalf("client_ip=%q subject=%q client=%q email=%q", clientIP, subject, client, email)
	}
}
