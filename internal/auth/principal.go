package auth

import "context"

type Kind string

const KindJWT Kind = "jwt"

const (
	ScopeExtractWrite = "extract:write"
	ScopeAdmin        = "admin"
)

type Principal struct {
	Subject string // OIDC sub (Keycloak user/service-account UUID)
	Client  string // OIDC azp (OAuth client id, e.g. pde-m2m / kalke-cli)
	Email   string // OIDC email when present (humans); empty for typical M2M
	Kind    Kind
	Scopes  []string
}

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
