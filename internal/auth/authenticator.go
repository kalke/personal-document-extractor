package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var ErrUnauthorized = errors.New("unauthorized")

type Authenticator struct {
	jwtEnabled       bool
	audience         string
	issuer           string
	jwks             keyfunc.Keyfunc
	introspectURL    string
	introspectSecret string
	http             *http.Client
}

type Options struct {
	Issuer           string // Expected JWT iss (public), e.g. http://localhost:8443/realms/kalke
	Audience         string
	DiscoveryURL     string // Optional fetch base when Issuer is not reachable (Docker → Caddy)
	IntrospectURL    string // Optional PAT introspection (kalke-auth)
	IntrospectSecret string
}

func NewAuthenticator(opts Options) (*Authenticator, error) {
	issuer := strings.TrimSpace(opts.Issuer)
	audience := strings.TrimSpace(opts.Audience)
	if issuer == "" || audience == "" {
		return nil, fmt.Errorf("OIDC_ISSUER and OIDC_AUDIENCE are required")
	}
	issuer = strings.TrimSuffix(issuer, "/")

	discovery := strings.TrimSuffix(strings.TrimSpace(opts.DiscoveryURL), "/")
	if discovery == "" {
		discovery = issuer
	}

	jwksURL, discoveredIssuer, err := discoverJWKS(discovery)
	if err != nil {
		return nil, err
	}
	// Keep configured issuer for token validation (public iss). Only adopt
	// discovery issuer when we discovered via the same URL as Issuer.
	if opts.DiscoveryURL == "" && discoveredIssuer != "" {
		issuer = strings.TrimSuffix(discoveredIssuer, "/")
	}
	// Keycloak often advertises jwks_uri on the public hostname (localhost);
	// rewrite to the reachable discovery origin inside Docker.
	if opts.DiscoveryURL != "" {
		rewritten, rewriteErr := rewriteURLOrigin(jwksURL, discovery)
		if rewriteErr != nil {
			return nil, fmt.Errorf("jwks url: %w", rewriteErr)
		}
		jwksURL = rewritten
	}

	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	return &Authenticator{
		jwtEnabled:       true,
		audience:         audience,
		issuer:           issuer,
		jwks:             k,
		introspectURL:    strings.TrimSpace(opts.IntrospectURL),
		introspectSecret: strings.TrimSpace(opts.IntrospectSecret),
		http:             &http.Client{Timeout: 10 * time.Second},
	}, nil
}

type oidcDiscovery struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

func discoverJWKS(base string) (jwksURL, discoveredIssuer string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/.well-known/openid-configuration", nil)
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

// rewriteURLOrigin replaces scheme://host[:port] of raw with that of originBase.
func rewriteURLOrigin(raw, originBase string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(originBase)
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid origin %q", originBase)
	}
	u.Scheme = base.Scheme
	u.Host = base.Host
	return u.String(), nil
}

func (a *Authenticator) Authenticate(ctx context.Context, bearerToken string) (Principal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" || !a.jwtEnabled {
		return Principal{}, ErrUnauthorized
	}
	if strings.HasPrefix(token, "kalke_") {
		return a.authenticatePAT(ctx, token)
	}
	return a.authenticateJWT(token)
}

type introspectResponse struct {
	Active      bool     `json:"active"`
	Sub         string   `json:"sub"`
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

func (a *Authenticator) authenticatePAT(ctx context.Context, token string) (Principal, error) {
	if a.introspectURL == "" || a.introspectSecret == "" {
		return Principal{}, ErrUnauthorized
	}
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.introspectURL, bytes.NewReader(body))
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kalke-Introspect-Key", a.introspectSecret)
	resp, err := a.http.Do(req)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Principal{}, ErrUnauthorized
	}
	var ir introspectResponse
	if err := json.NewDecoder(resp.Body).Decode(&ir); err != nil || !ir.Active || ir.Sub == "" {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		Subject: ir.Sub,
		Email:   ir.Email,
		Client:  "kalke-pat",
		Kind:    KindPAT,
		Scopes:  scopesFromClaims(ir.Permissions, ""),
	}, nil
}

type oidcClaims struct {
	Permissions []string `json:"permissions"`
	Scope       string   `json:"scope"`
	Email       string   `json:"email"`
	Azp         string   `json:"azp"`
	jwt.RegisteredClaims
}

func (a *Authenticator) authenticateJWT(tokenStr string) (Principal, error) {
	claims := &oidcClaims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, a.jwks.Keyfunc,
		jwt.WithAudience(a.audience),
		jwt.WithIssuer(a.issuer),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
	)
	if err != nil || !parsed.Valid {
		return Principal{}, ErrUnauthorized
	}
	sub := claims.Subject
	if sub == "" {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		Subject: sub,
		Client:  strings.TrimSpace(claims.Azp),
		Email:   strings.TrimSpace(claims.Email),
		Kind:    KindJWT,
		Scopes:  scopesFromClaims(claims.Permissions, claims.Scope),
	}, nil
}

// scopesFromClaims prefers the permissions claim, then space-separated scope.
// Empty claims fail closed (no implicit extract:write).
func scopesFromClaims(permissions []string, scope string) []string {
	if len(permissions) > 0 {
		return append([]string(nil), permissions...)
	}
	if scope != "" {
		return strings.Fields(scope)
	}
	return nil
}
