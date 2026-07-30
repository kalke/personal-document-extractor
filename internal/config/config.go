package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port          string
	GroqAPIKey    string
	GroqModel     string
	GroqBaseURL   string
	LogLevel      string
	LogFormat     string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisCacheTTL time.Duration
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

	cfg := Config{
		Port:          getenv("PORT", "8080"),
		GroqAPIKey:    strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
		GroqModel:     getenv("GROQ_MODEL", "qwen/qwen3.6-27b"),
		GroqBaseURL:   getenv("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
		LogLevel:      strings.ToLower(getenv("LOG_LEVEL", "info")),
		LogFormat:     strings.ToLower(getenv("LOG_FORMAT", "text")),
		DatabaseURL:   strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisAddr:     getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDB,
		RedisCacheTTL: ttl,
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

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
