package authz

import "github.com/kalke/personal-document-extractor/internal/auth"

func HasScope(p auth.Principal, want string) bool {
	for _, s := range p.Scopes {
		if s == auth.ScopeAdmin || s == want {
			return true
		}
	}
	return false
}
