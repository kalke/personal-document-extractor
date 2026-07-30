package httpapi

import (
	"context"

	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/preprocess"
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

type DBPinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Extractor   Extractor
	Pool        DBPinger
	Cache       ResultCache
	Extractions ExtractionStore
}
