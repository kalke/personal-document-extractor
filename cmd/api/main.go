package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/kalke/personal-document-extractor/internal/config"
	"github.com/kalke/personal-document-extractor/internal/doctypes/address_proof"
	"github.com/kalke/personal-document-extractor/internal/doctypes/identity_document"
	"github.com/kalke/personal-document-extractor/internal/doctypes/invoice_nf"
	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/httpapi"
	"github.com/kalke/personal-document-extractor/internal/llm/groq"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	setupLogger(cfg.LogLevel, cfg.LogFormat)

	llm := groq.New(cfg.GroqAPIKey, cfg.GroqModel, cfg.GroqBaseURL)
	svc := extract.NewService(
		llm,
		address_proof.DocType{},
		identity_document.DocType{},
		invoice_nf.DocType{},
	)

	addr := ":" + cfg.Port
	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.New(svc),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      130 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("server starting", "addr", addr, "model", cfg.GroqModel)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func setupLogger(level, format string) {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lv}
	var h slog.Handler
	if strings.EqualFold(format, "json") {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}
