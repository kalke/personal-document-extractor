package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	GroqAPIKey         string
	GroqModel          string
	GroqBaseURL        string
	LogLevel           string
	LogFormat          string
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	RedisCacheTTL      time.Duration
	TrustedProxies     []*net.IPNet
	OIDCIssuer         string
	OIDCAudience       string
	OIDCDiscoveryURL   string
	RateLimitPerMinute int
}

func Load() (Config, error) {
	ttl, err := time.ParseDuration(getenv("REDIS_CACHE_TTL", "24h"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid REDIS_CACHE_TTL: %w", err)
	}
	if ttl <= 0 {
		return Config{}, fmt.Errorf("invalid REDIS_CACHE_TTL: must be > 0")
	}

	redisDB, err := strconv.Atoi(getenv("REDIS_DB", "0"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid REDIS_DB: %w", err)
	}
	if redisDB < 0 {
		return Config{}, fmt.Errorf("invalid REDIS_DB: must be >= 0")
	}

	rateLimit, err := strconv.Atoi(getenv("RATE_LIMIT_PER_MINUTE", "60"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid RATE_LIMIT_PER_MINUTE: %w", err)
	}
	if rateLimit <= 0 {
		return Config{}, fmt.Errorf("invalid RATE_LIMIT_PER_MINUTE: must be > 0")
	}

	trusted, err := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return Config{}, err
	}

	oidcIssuer := strings.TrimSpace(os.Getenv("OIDC_ISSUER"))
	oidcAudience := strings.TrimSpace(os.Getenv("OIDC_AUDIENCE"))
	if oidcIssuer == "" || oidcAudience == "" {
		return Config{}, fmt.Errorf("OIDC_ISSUER and OIDC_AUDIENCE are required")
	}

	cfg := Config{
		Port:               getenv("PORT", "8080"),
		GroqAPIKey:         strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
		GroqModel:          getenv("GROQ_MODEL", "qwen/qwen3.6-27b"),
		GroqBaseURL:        getenv("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
		LogLevel:           strings.ToLower(getenv("LOG_LEVEL", "info")),
		LogFormat:          strings.ToLower(getenv("LOG_FORMAT", "text")),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisAddr:          getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      os.Getenv("REDIS_PASSWORD"),
		RedisDB:            redisDB,
		RedisCacheTTL:      ttl,
		TrustedProxies:     trusted,
		OIDCIssuer:         oidcIssuer,
		OIDCAudience:       oidcAudience,
		OIDCDiscoveryURL:   strings.TrimSpace(os.Getenv("OIDC_DISCOVERY_URL")),
		RateLimitPerMinute: rateLimit,
	}
	if cfg.GroqAPIKey == "" {
		return Config{}, fmt.Errorf("GROQ_API_KEY is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("invalid PORT: %s", cfg.Port)
	}

	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("invalid LOG_LEVEL: %s", cfg.LogLevel)
	}
	switch cfg.LogFormat {
	case "text", "json":
	default:
		return Config{}, fmt.Errorf("invalid LOG_FORMAT: %s", cfg.LogFormat)
	}
	return cfg, nil
}

// parseTrustedProxies accepts a comma-separated list of CIDRs or bare IPs.
// Empty means client IP headers are never trusted.
func parseTrustedProxies(raw string) ([]*net.IPNet, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			ip := net.ParseIP(part)
			if ip == nil {
				return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry: %s", part)
			}
			if v4 := ip.To4(); v4 != nil {
				part = v4.String() + "/32"
			} else {
				part = ip.String() + "/128"
			}
		}
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry: %s", part)
		}
		out = append(out, network)
	}
	return out, nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
