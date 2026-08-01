package cache

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(addr, password string, db int, ttl time.Duration, useTLS bool) *Cache {
	opts := &redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	}
	if useTLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	rdb := redis.NewClient(opts)
	return &Cache{rdb: rdb, ttl: ttl}
}

func (c *Cache) Ping(ctx context.Context) error {
	if c == nil || c.rdb == nil {
		return errors.New("redis not configured")
	}
	return c.rdb.Ping(ctx).Err()
}

func (c *Cache) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// Redis exposes the underlying client for shared infrastructure (e.g. rate limiting).
func (c *Cache) Redis() *redis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}

func Key(docType, sha256hex string) string {
	return fmt.Sprintf("extract:v1:%s:%s", docType, sha256hex)
}

func (c *Cache) Get(ctx context.Context, docType, sha256hex string) (extract.Result, bool) {
	if c == nil || c.rdb == nil {
		return extract.Result{}, false
	}
	raw, err := c.rdb.Get(ctx, Key(docType, sha256hex)).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("redis get failed; continuing without cache", "err", err)
		}
		return extract.Result{}, false
	}
	var result extract.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		key := Key(docType, sha256hex)
		slog.Warn("redis cache decode failed; deleting key", "err", err, "key", key)
		if delErr := c.rdb.Del(ctx, key).Err(); delErr != nil {
			slog.Warn("redis cache delete failed", "err", delErr, "key", key)
		}
		return extract.Result{}, false
	}
	return result, true
}

func (c *Cache) Set(ctx context.Context, docType, sha256hex string, result extract.Result) {
	if c == nil || c.rdb == nil {
		return
	}
	raw, err := json.Marshal(result)
	if err != nil {
		slog.Warn("redis cache encode failed", "err", err)
		return
	}
	if err := c.rdb.Set(ctx, Key(docType, sha256hex), raw, c.ttl).Err(); err != nil {
		slog.Warn("redis set failed; continuing without cache", "err", err)
	}
}

func (c *Cache) Delete(ctx context.Context, docType, sha256hex string) {
	if c == nil || c.rdb == nil {
		return
	}
	if err := c.rdb.Del(ctx, Key(docType, sha256hex)).Err(); err != nil {
		slog.Warn("redis delete failed; continuing without cache", "err", err)
	}
}
