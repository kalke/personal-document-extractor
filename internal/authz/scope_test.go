package authz_test

import (
	"testing"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/authz"
)

func TestHasScope(t *testing.T) {
	if !authz.HasScope(auth.Principal{Scopes: []string{auth.ScopeExtractWrite}}, auth.ScopeExtractWrite) {
		t.Fatal("expected extract:write")
	}
	if !authz.HasScope(auth.Principal{Scopes: []string{auth.ScopeAdmin}}, auth.ScopeExtractWrite) {
		t.Fatal("admin should imply extract:write")
	}
	if authz.HasScope(auth.Principal{Scopes: []string{"openid"}}, auth.ScopeExtractWrite) {
		t.Fatal("unrelated scope should not imply extract:write")
	}
}
