package auth

import (
	"reflect"
	"testing"
)

func TestScopesFromClaims(t *testing.T) {
	cases := []struct {
		name        string
		permissions []string
		scope       string
		want        []string
	}{
		{
			name:        "permissions_win",
			permissions: []string{ScopeExtractWrite, ScopeAdmin},
			scope:       "openid profile",
			want:        []string{ScopeExtractWrite, ScopeAdmin},
		},
		{
			name:  "scope_fields",
			scope: "extract:write openid",
			want:  []string{ScopeExtractWrite, "openid"},
		},
		{
			name: "empty_fails_closed",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scopesFromClaims(tc.permissions, tc.scope)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestHasScope(t *testing.T) {
	admin := Principal{Scopes: []string{ScopeAdmin}}
	if !admin.HasScope(ScopeExtractWrite) {
		t.Fatal("admin should imply extract:write")
	}
	writer := Principal{Scopes: []string{ScopeExtractWrite}}
	if !writer.HasScope(ScopeExtractWrite) {
		t.Fatal("extract:write should allow extract:write")
	}
	openid := Principal{Scopes: []string{"openid"}}
	if openid.HasScope(ScopeExtractWrite) {
		t.Fatal("openid alone should not allow extract:write")
	}
}

func TestNewAuthenticatorRequiresOIDC(t *testing.T) {
	if _, err := NewAuthenticator(Options{}); err == nil {
		t.Fatal("expected error when issuer/audience empty")
	}
	if _, err := NewAuthenticator(Options{Issuer: "http://localhost:8443/realms/kalke"}); err == nil {
		t.Fatal("expected error when only issuer set")
	}
}
