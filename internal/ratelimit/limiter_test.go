package ratelimit_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/kalke/personal-document-extractor/internal/ratelimit"
)

func TestLimiterAllowAndBlock(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	l := ratelimit.New(rdb, 2)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		res, err := l.Allow(ctx, "api_key", "p1")
		if err != nil || !res.Allowed {
			t.Fatalf("i=%d res=%+v err=%v", i, res, err)
		}
	}
	res, err := l.Allow(ctx, "api_key", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed || res.Remaining != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestLimiterFailClosed(t *testing.T) {
	l := ratelimit.New(nil, 10)
	if _, err := l.Allow(context.Background(), "api_key", "x"); err == nil {
		t.Fatal("expected unavailable")
	}
}
