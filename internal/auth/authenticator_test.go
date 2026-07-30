package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/kalke/personal-document-extractor/internal/auth"
)

type memKeys struct {
	byPrefix map[string]auth.APIKeyRecord
}

func (m *memKeys) LookupByPrefix(_ context.Context, prefix string) (auth.APIKeyRecord, error) {
	rec, ok := m.byPrefix[prefix]
	if !ok {
		return auth.APIKeyRecord{}, auth.ErrUnauthorized
	}
	return rec, nil
}

func TestAuthenticatorAPIKey(t *testing.T) {
	plain, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	a, err := auth.NewAuthenticator(auth.Options{
		Keys: &memKeys{byPrefix: map[string]auth.APIKeyRecord{
			prefix: {
				ID:        "11111111-1111-1111-1111-111111111111",
				UserID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
				KeyPrefix: prefix,
				KeyHash:   hash,
				Scopes:    []string{auth.ScopeExtractWrite},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Authenticate(context.Background(), plain)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != auth.KindAPIKey || p.APIKeyID == "" || p.UserID == "" || len(p.Scopes) != 1 {
		t.Fatalf("%+v", p)
	}

	if _, err := a.Authenticate(context.Background(), "pde_live_deadbeef12_nosuch"); err == nil {
		t.Fatal("expected unauthorized")
	}
}

func TestAuthenticatorRejectsRevokedAndExpired(t *testing.T) {
	plain, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	revoked := time.Now()
	expired := time.Now().Add(-time.Hour)
	cases := []auth.APIKeyRecord{
		{ID: "1", KeyPrefix: prefix, KeyHash: hash, Scopes: []string{auth.ScopeExtractWrite}, RevokedAt: &revoked},
		{ID: "2", KeyPrefix: prefix, KeyHash: hash, Scopes: []string{auth.ScopeExtractWrite}, ExpiresAt: &expired},
	}
	for _, rec := range cases {
		a, err := auth.NewAuthenticator(auth.Options{
			Keys: &memKeys{byPrefix: map[string]auth.APIKeyRecord{prefix: rec}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Authenticate(context.Background(), plain); err == nil {
			t.Fatalf("expected reject for %+v", rec)
		}
	}
}

func TestAuthenticatorAuth0PairRequired(t *testing.T) {
	_, err := auth.NewAuthenticator(auth.Options{Keys: &memKeys{}, Domain: "x.auth0.com"})
	if err == nil {
		t.Fatal("expected error when only domain set")
	}
}
