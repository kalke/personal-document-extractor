package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kalke/personal-document-extractor/internal/db"
	"github.com/kalke/personal-document-extractor/internal/store"
)

func TestUsersUpsertIdempotentIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	users := store.NewUsers(pool)
	u1, err := users.UpsertBySubject(ctx, store.UpsertUserInput{
		AuthSubject:   "oidc|upsert-test",
		Email:         "a@example.com",
		EmailVerified: true,
		DisplayName:   "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	u2, err := users.UpsertBySubject(ctx, store.UpsertUserInput{
		AuthSubject:   "oidc|upsert-test",
		Email:         "b@example.com",
		EmailVerified: true,
		DisplayName:   "B",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID != u2.ID {
		t.Fatalf("ids differ %s vs %s", u1.ID, u2.ID)
	}
	if u2.Email != "b@example.com" || u2.DisplayName != "B" {
		t.Fatalf("%+v", u2)
	}

	ops, err := users.EnsureSystemOps(ctx)
	if err != nil || ops.AuthSubject != store.SystemOpsSubject {
		t.Fatalf("ops=%+v err=%v", ops, err)
	}
}
