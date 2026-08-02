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

func TestIsAllowlistedAdmin(t *testing.T) {
	allow := []string{"henriquekalke@icloud.com"}
	ok := auth.Principal{
		Email:  "henriquekalke@icloud.com",
		Scopes: []string{auth.ScopeAdmin},
	}
	if !authz.IsAllowlistedAdmin(ok, allow) {
		t.Fatal("expected allowlisted admin")
	}
	if authz.IsAllowlistedAdmin(auth.Principal{
		Email:  "other@example.com",
		Scopes: []string{auth.ScopeAdmin},
	}, allow) {
		t.Fatal("non-allowlisted email must fail")
	}
	if authz.IsAllowlistedAdmin(auth.Principal{
		Email:  "henriquekalke@icloud.com",
		Scopes: []string{auth.ScopeExtractWrite},
	}, allow) {
		t.Fatal("extract:write must not count as admin")
	}
	if authz.IsAllowlistedAdmin(auth.Principal{
		Email:  "",
		Scopes: []string{auth.ScopeAdmin},
	}, allow) {
		t.Fatal("empty email must fail")
	}
}
