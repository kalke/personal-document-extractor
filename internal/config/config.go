package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port        string
	GroqAPIKey  string
	GroqModel   string
	GroqBaseURL string
	LogLevel    string
	LogFormat   string
}

func Load() (Config, error) {
	cfg := Config{
		Port:        getenv("PORT", "8080"),
		GroqAPIKey:  os.Getenv("GROQ_API_KEY"),
		GroqModel:   getenv("GROQ_MODEL", "qwen/qwen3.6-27b"),
		GroqBaseURL: getenv("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
		LogLevel:    strings.ToLower(getenv("LOG_LEVEL", "info")),
		LogFormat:   strings.ToLower(getenv("LOG_FORMAT", "text")),
	}
	if cfg.GroqAPIKey == "" {
		return Config{}, fmt.Errorf("GROQ_API_KEY is required")
	}
	if _, err := strconv.Atoi(cfg.Port); err != nil {
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
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
