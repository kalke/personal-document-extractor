package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port       string
	GroqAPIKey string
	GroqModel  string
	GroqBaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		Port:        getenv("PORT", "8080"),
		GroqAPIKey:  os.Getenv("GROQ_API_KEY"),
		GroqModel:   getenv("GROQ_MODEL", "llama-3.3-70b-versatile"),
		GroqBaseURL: getenv("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
	}
	if cfg.GroqAPIKey == "" {
		return Config{}, fmt.Errorf("GROQ_API_KEY is required")
	}
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return Config{}, fmt.Errorf("invalid PORT: %s", cfg.Port)
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
