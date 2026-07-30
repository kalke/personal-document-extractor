package auth

import "context"

type Kind string

const (
	KindAPIKey Kind = "api_key"
	KindJWT    Kind = "jwt"
)

const (
	ScopeExtractWrite = "extract:write"
	ScopeKeysManage   = "keys:manage"
	ScopeAdmin        = "admin"
)

type Principal struct {
	Subject  string
	Kind     Kind
	APIKeyID string
	Scopes   []string
}

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
