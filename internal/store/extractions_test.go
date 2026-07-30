package store_test

import (
	"context"
	"testing"

	"github.com/kalke/personal-document-extractor/internal/store"
)

func TestNilStorePersist(t *testing.T) {
	var s *store.Extractions
	rec := store.ExtractionRecord{Status: "success"}
	if err := s.Insert(context.Background(), rec); err == nil {
		t.Fatal("expected insert error")
	}
	if err := s.Replace(context.Background(), rec); err == nil {
		t.Fatal("expected replace error")
	}
}
