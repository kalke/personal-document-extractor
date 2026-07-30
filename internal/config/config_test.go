package config_test

import (
	"net"
	"testing"
	"time"

	"github.com/kalke/personal-document-extractor/internal/config"
)

func withOIDC(t *testing.T) {
	t.Helper()
	t.Setenv("OIDC_ISSUER", "http://localhost:8443/realms/kalke")
	t.Setenv("OIDC_AUDIENCE", "personal-document-extractor")
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "test-key")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("PORT", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_CACHE_TTL", "")
	t.Setenv("REDIS_DB", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("GROQ_MODEL", "")
	t.Setenv("TRUSTED_PROXIES", "")
	withOIDC(t)
	t.Setenv("RATE_LIMIT_PER_MINUTE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("Port: got %q", cfg.Port)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Fatalf("RedisAddr: got %q", cfg.RedisAddr)
	}
	if cfg.RedisCacheTTL != 24*time.Hour {
		t.Fatalf("RedisCacheTTL: got %v", cfg.RedisCacheTTL)
	}
	if cfg.RateLimitPerMinute != 60 {
		t.Fatalf("RateLimitPerMinute: got %d", cfg.RateLimitPerMinute)
	}
	if cfg.LogLevel != "info" || cfg.LogFormat != "text" {
		t.Fatalf("log defaults: %q %q", cfg.LogLevel, cfg.LogFormat)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("TrustedProxies: got %v", cfg.TrustedProxies)
	}
}

func TestLoadTrustedProxies(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "k")
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("PORT", "8080")
	t.Setenv("REDIS_DB", "0")
	t.Setenv("REDIS_CACHE_TTL", "24h")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "text")
	withOIDC(t)
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.1")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Fatalf("len=%d", len(cfg.TrustedProxies))
	}
	if !cfg.TrustedProxies[0].Contains(net.ParseIP("10.1.2.3")) {
		t.Fatal("expected 10.0.0.0/8")
	}
	if !cfg.TrustedProxies[1].Contains(net.ParseIP("192.168.1.1")) {
		t.Fatal("expected 192.168.1.1/32")
	}

	t.Setenv("TRUSTED_PROXIES", "not-an-ip")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected invalid TRUSTED_PROXIES error")
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	withOIDC(t)
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected error for missing GROQ_API_KEY")
	}

	t.Setenv("GROQ_API_KEY", "k")
	t.Setenv("DATABASE_URL", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}

	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("OIDC_AUDIENCE", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected error for missing OIDC")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	base := func() {
		t.Setenv("GROQ_API_KEY", "k")
		t.Setenv("DATABASE_URL", "postgres://localhost/db")
		t.Setenv("PORT", "8080")
		t.Setenv("REDIS_DB", "0")
		t.Setenv("REDIS_CACHE_TTL", "24h")
		t.Setenv("LOG_LEVEL", "info")
		t.Setenv("LOG_FORMAT", "text")
		t.Setenv("OIDC_ISSUER", "http://localhost:8443/realms/kalke")
		t.Setenv("OIDC_AUDIENCE", "personal-document-extractor")
		t.Setenv("RATE_LIMIT_PER_MINUTE", "60")
		t.Setenv("TRUSTED_PROXIES", "")
	}

	cases := []struct {
		name string
		set  func()
	}{
		{"port", func() { base(); t.Setenv("PORT", "abc") }},
		{"port_zero", func() { base(); t.Setenv("PORT", "0") }},
		{"port_high", func() { base(); t.Setenv("PORT", "70000") }},
		{"ttl", func() { base(); t.Setenv("REDIS_CACHE_TTL", "nope") }},
		{"ttl_zero", func() { base(); t.Setenv("REDIS_CACHE_TTL", "0s") }},
		{"redis_db", func() { base(); t.Setenv("REDIS_DB", "x") }},
		{"redis_db_neg", func() { base(); t.Setenv("REDIS_DB", "-1") }},
		{"log_level", func() { base(); t.Setenv("LOG_LEVEL", "verbose") }},
		{"log_format", func() { base(); t.Setenv("LOG_FORMAT", "yaml") }},
		{"blank_key", func() { base(); t.Setenv("GROQ_API_KEY", "   ") }},
		{"trusted_proxies", func() { base(); t.Setenv("TRUSTED_PROXIES", "10/bad") }},
		{"oidc_missing_audience", func() {
			base()
			t.Setenv("OIDC_ISSUER", "http://localhost:8443/realms/kalke")
			t.Setenv("OIDC_AUDIENCE", "")
		}},
		{"rate_limit", func() { base(); t.Setenv("RATE_LIMIT_PER_MINUTE", "0") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.set()
			if _, err := config.Load(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
