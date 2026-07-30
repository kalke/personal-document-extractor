package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/kalke/personal-document-extractor/internal/cache"
	"github.com/kalke/personal-document-extractor/internal/extract"
)

func TestKey(t *testing.T) {
	got := cache.Key("identity_document", "abc")
	want := "extract:v1:identity_document:abc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNilCacheFailOpen(t *testing.T) {
	var c *cache.Cache
	if _, ok := c.Get(context.Background(), "t", "sha"); ok {
		t.Fatal("nil cache Get should miss")
	}
	c.Set(context.Background(), "t", "sha", extract.Result{})
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("nil cache Ping should error")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
}

func TestGetSetRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	in := extract.Result{
		DocType: "identity_document",
		Data:    map[string]any{"nome": "FULANO DA SILVA"},
		Meta:    extract.Meta{Model: "test-model", Mode: "vision"},
	}
	c.Set(ctx, "identity_document", "deadbeef", in)

	got, ok := c.Get(ctx, "identity_document", "deadbeef")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.DocType != in.DocType || got.Meta.Model != "test-model" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if _, ok := c.Get(ctx, "identity_document", "missing"); ok {
		t.Fatal("expected miss")
	}
}

func TestDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	c.Set(ctx, "identity_document", "abc", extract.Result{DocType: "identity_document"})
	c.Delete(ctx, "identity_document", "abc")
	if _, ok := c.Get(ctx, "identity_document", "abc"); ok {
		t.Fatal("expected miss after delete")
	}
}

func TestCorruptCacheDeleted(t *testing.T) {
	mr := miniredis.RunT(t)
	c := cache.New(mr.Addr(), "", 0, time.Hour)
	t.Cleanup(func() { _ = c.Close() })

	key := cache.Key("identity_document", "bad")
	if err := mr.Set(key, "{not-json"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(context.Background(), "identity_document", "bad"); ok {
		t.Fatal("expected miss")
	}
	if mr.Exists(key) {
		t.Fatal("corrupt key should be deleted")
	}
}
