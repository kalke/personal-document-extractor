package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var ErrUnauthorized = errors.New("unauthorized")

// UserSync upserts the local users projection for JWT principals.
type UserSync interface {
	UpsertFromAuth(ctx context.Context, in UserSyncInput) (userID string, err error)
}

type UserSyncInput struct {
	Subject       string
	Email         string
	EmailVerified bool
	DisplayName   string
}

type Authenticator struct {
	users      UserSync
	jwtEnabled bool
	audience   string
	issuer     string
	jwks       keyfunc.Keyfunc
}

type Options struct {
	Users    UserSync // optional; when set, JWT auth upserts local users
	Issuer   string   // OIDC issuer URL, e.g. http://localhost:8443/realms/kalke
	Audience string
}

func NewAuthenticator(opts Options) (*Authenticator, error) {
	issuer := strings.TrimSpace(opts.Issuer)
	audience := strings.TrimSpace(opts.Audience)
	if issuer == "" || audience == "" {
		return nil, fmt.Errorf("OIDC_ISSUER and OIDC_AUDIENCE are required")
	}
	issuer = strings.TrimSuffix(issuer, "/")
	jwksURL, discoveredIssuer, err := discoverJWKS(issuer)
	if err != nil {
		return nil, err
	}
	if discoveredIssuer != "" {
		issuer = strings.TrimSuffix(discoveredIssuer, "/")
	}
	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	return &Authenticator{
		users:      opts.Users,
		jwtEnabled: true,
		audience:   audience,
		issuer:     issuer,
		jwks:       k,
	}, nil
}

type oidcDiscovery struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

func discoverJWKS(issuer string) (jwksURL, discoveredIssuer string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return "", "", fmt.Errorf("oidc discovery: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("oidc discovery: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("oidc discovery: unexpected status %d", resp.StatusCode)
	}
	var doc oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", "", fmt.Errorf("oidc discovery: %w", err)
	}
	if doc.JWKSURI == "" {
		return "", "", fmt.Errorf("oidc discovery: missing jwks_uri")
	}
	return doc.JWKSURI, doc.Issuer, nil
}

func (a *Authenticator) Authenticate(ctx context.Context, bearerToken string) (Principal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return Principal{}, ErrUnauthorized
	}
	if !a.jwtEnabled {
		return Principal{}, ErrUnauthorized
	}
	return a.authenticateJWT(ctx, token)
}

type oidcClaims struct {
	Permissions   []string `json:"permissions"`
	Scope         string   `json:"scope"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Name          string   `json:"name"`
	jwt.RegisteredClaims
}

func (a *Authenticator) authenticateJWT(ctx context.Context, tokenStr string) (Principal, error) {
	claims := &oidcClaims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, a.jwks.Keyfunc,
		jwt.WithAudience(a.audience),
		jwt.WithIssuer(a.issuer),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
	)
	if err != nil || !parsed.Valid {
		return Principal{}, ErrUnauthorized
	}
	scopes := scopesFromClaims(claims.Permissions, claims.Scope)
	sub := claims.Subject
	if sub == "" {
		return Principal{}, ErrUnauthorized
	}
	p := Principal{
		Subject: sub,
		Kind:    KindJWT,
		Scopes:  scopes,
	}
	if a.users != nil {
		userID, err := a.users.UpsertFromAuth(ctx, UserSyncInput{
			Subject:       sub,
			Email:         claims.Email,
			EmailVerified: claims.EmailVerified,
			DisplayName:   claims.Name,
		})
		if err != nil {
			return Principal{}, ErrUnauthorized
		}
		p.UserID = userID
	}
	return p, nil
}

// scopesFromClaims prefers the permissions claim, then space-separated scope.
// When neither is present, grants extract:write (bootstrap without IdP RBAC).
func scopesFromClaims(permissions []string, scope string) []string {
	if len(permissions) > 0 {
		return append([]string(nil), permissions...)
	}
	if scope != "" {
		return strings.Fields(scope)
	}
	return []string{ScopeExtractWrite}
}
