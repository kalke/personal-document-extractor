package auth_test

import (
	"testing"

	"github.com/kalke/personal-document-extractor/internal/auth"
)

func TestGenerateAndLookupAPIKey(t *testing.T) {
	plain, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if hash != auth.HashAPIKey(plain) {
		t.Fatal("hash mismatch")
	}
	got, ok := auth.LookupPrefix(plain)
	if !ok || got != prefix {
		t.Fatalf("prefix=%q got=%q ok=%v", prefix, got, ok)
	}
	if _, ok := auth.LookupPrefix("not-a-key"); ok {
		t.Fatal("expected false")
	}
}

func TestParseScopesCSV(t *testing.T) {
	scopes, err := auth.ParseScopesCSV("")
	if err != nil || len(scopes) != 1 || scopes[0] != auth.ScopeExtractWrite {
		t.Fatalf("default: %v %v", scopes, err)
	}
	scopes, err = auth.ParseScopesCSV("extract:write,admin")
	if err != nil || len(scopes) != 2 {
		t.Fatalf("got %v %v", scopes, err)
	}
	if _, err := auth.ParseScopesCSV("nope"); err == nil {
		t.Fatal("expected error")
	}
	scopes, err = auth.ParseScopesCSV(auth.ScopeAdmin)
	if err != nil || len(scopes) != 1 || scopes[0] != auth.ScopeAdmin {
		t.Fatalf("admin: %v %v", scopes, err)
	}
}
