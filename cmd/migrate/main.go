package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/joho/godotenv"

	"github.com/kalke/personal-document-extractor/internal/migrate"
)

func main() {
	_ = godotenv.Load()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	searchPath := os.Getenv("DB_SEARCH_PATH")
	if searchPath == "" {
		searchPath = "pde"
	}
	if err := migrate.Up(context.Background(), databaseURL, searchPath); err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}
