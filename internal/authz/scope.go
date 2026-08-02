package authz

import (
	"strings"

	"github.com/kalke/personal-document-extractor/internal/auth"
)

func HasScope(p auth.Principal, want string) bool {
	for _, s := range p.Scopes {
		if s == auth.ScopeAdmin || s == want {
			return true
		}
	}
	return false
}

func HasExactScope(p auth.Principal, want string) bool {
	for _, s := range p.Scopes {
		if s == want {
			return true
		}
	}
	return false
}

// IsAllowlistedAdmin requires the admin scope and an email on the allowlist.
func IsAllowlistedAdmin(p auth.Principal, adminEmails []string) bool {
	if !HasExactScope(p, auth.ScopeAdmin) {
		return false
	}
	email := strings.ToLower(strings.TrimSpace(p.Email))
	if email == "" {
		return false
	}
	for _, allowed := range adminEmails {
		if email == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}
