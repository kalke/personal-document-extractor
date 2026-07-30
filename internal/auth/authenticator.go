package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

type APIKeyRecord struct {
	ID        string
	UserID    string
	Name      string
	KeyPrefix string
	KeyHash   string
	Scopes    []string
	ExpiresAt *time.Time
	RevokedAt *time.Time
}

type APIKeyLookup interface {
	LookupByPrefix(ctx context.Context, prefix string) (APIKeyRecord, error)
}

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
	keys       APIKeyLookup
	users      UserSync
	jwtEnabled bool
	audience   string
	issuer     string
	jwks       keyfunc.Keyfunc
}

type Options struct {
	Keys     APIKeyLookup
	Users    UserSync // optional; when set, JWT auth upserts local users
	Domain   string   // Auth0 domain, e.g. tenant.auth0.com
	Audience string
}

func NewAuthenticator(opts Options) (*Authenticator, error) {
	if opts.Keys == nil {
		return nil, fmt.Errorf("api key lookup is required")
	}
	a := &Authenticator{keys: opts.Keys, users: opts.Users}
	domain := strings.TrimSpace(opts.Domain)
	audience := strings.TrimSpace(opts.Audience)
	if domain == "" && audience == "" {
		return a, nil
	}
	if domain == "" || audience == "" {
		return nil, fmt.Errorf("AUTH0_DOMAIN and AUTH0_AUDIENCE must both be set or both empty")
	}
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimSuffix(domain, "/")
	jwksURL := fmt.Sprintf("https://%s/.well-known/jwks.json", domain)
	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	a.jwtEnabled = true
	a.audience = audience
	a.issuer = "https://" + domain + "/"
	a.jwks = k
	return a, nil
}

func (a *Authenticator) Authenticate(ctx context.Context, bearerToken string) (Principal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return Principal{}, ErrUnauthorized
	}
	if strings.HasPrefix(token, APIKeyPrefix) {
		return a.authenticateAPIKey(ctx, token)
	}
	if a.jwtEnabled {
		return a.authenticateJWT(ctx, token)
	}
	return Principal{}, ErrUnauthorized
}

func (a *Authenticator) authenticateAPIKey(ctx context.Context, plaintext string) (Principal, error) {
	prefix, ok := LookupPrefix(plaintext)
	if !ok {
		return Principal{}, ErrUnauthorized
	}
	rec, err := a.keys.LookupByPrefix(ctx, prefix)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	if rec.RevokedAt != nil {
		return Principal{}, ErrUnauthorized
	}
	if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
		return Principal{}, ErrUnauthorized
	}
	if rec.KeyHash != HashAPIKey(plaintext) {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		Subject:  "api_key:" + rec.ID,
		Kind:     KindAPIKey,
		UserID:   rec.UserID,
		APIKeyID: rec.ID,
		Scopes:   append([]string(nil), rec.Scopes...),
	}, nil
}

type auth0Claims struct {
	Permissions   []string `json:"permissions"`
	Scope         string   `json:"scope"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Name          string   `json:"name"`
	jwt.RegisteredClaims
}

func (a *Authenticator) authenticateJWT(ctx context.Context, tokenStr string) (Principal, error) {
	claims := &auth0Claims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, a.jwks.Keyfunc,
		jwt.WithAudience(a.audience),
		jwt.WithIssuer(a.issuer),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
	)
	if err != nil || !parsed.Valid {
		return Principal{}, ErrUnauthorized
	}
	scopes := claims.Permissions
	if len(scopes) == 0 && claims.Scope != "" {
		scopes = strings.Fields(claims.Scope)
	}
	// When Auth0 RBAC permissions are not configured yet, grant self-service defaults.
	if len(scopes) == 0 {
		scopes = []string{ScopeExtractWrite, ScopeKeysManage}
	}
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
