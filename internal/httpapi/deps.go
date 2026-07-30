package httpapi

import (
	"context"
	"net"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/preprocess"
	"github.com/kalke/personal-document-extractor/internal/ratelimit"
	"github.com/kalke/personal-document-extractor/internal/store"
)

type Extractor interface {
	KnownTypes() []string
	Extract(ctx context.Context, docType string, doc preprocess.PreparedDocument) (extract.Result, error)
}

type ResultCache interface {
	Ping(ctx context.Context) error
	Get(ctx context.Context, docType, sha256hex string) (extract.Result, bool)
	Set(ctx context.Context, docType, sha256hex string, result extract.Result)
	Delete(ctx context.Context, docType, sha256hex string)
}

type ExtractionStore interface {
	Insert(ctx context.Context, rec store.ExtractionRecord) error
	Replace(ctx context.Context, rec store.ExtractionRecord) error
}

type UserStore interface {
	GetByID(ctx context.Context, id string) (store.User, error)
}

type APIKeyStore interface {
	Create(ctx context.Context, in store.CreateAPIKeyInput) (auth.APIKeyRecord, error)
	ListByUser(ctx context.Context, userID string) ([]store.APIKeyPublic, error)
	RevokeForUser(ctx context.Context, userID, keyID string) error
}

type DBPinger interface {
	Ping(ctx context.Context) error
}

type Authenticator interface {
	Authenticate(ctx context.Context, bearerToken string) (auth.Principal, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, kind, principalID string) (ratelimit.Result, error)
}

type Deps struct {
	Extractor      Extractor
	Pool           DBPinger
	Cache          ResultCache
	Extractions    ExtractionStore
	Users          UserStore
	APIKeys        APIKeyStore
	TrustedProxies []*net.IPNet
	Auth           Authenticator
	RateLimit      RateLimiter
	RequiredScope  string
}
