package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/cache"
	"github.com/kalke/personal-document-extractor/internal/config"
	"github.com/kalke/personal-document-extractor/internal/db"
	"github.com/kalke/personal-document-extractor/internal/doctypes/address_proof"
	"github.com/kalke/personal-document-extractor/internal/doctypes/identity_document"
	"github.com/kalke/personal-document-extractor/internal/doctypes/invoice_nf"
	"github.com/kalke/personal-document-extractor/internal/extract"
	"github.com/kalke/personal-document-extractor/internal/httpapi"
	"github.com/kalke/personal-document-extractor/internal/identity"
	"github.com/kalke/personal-document-extractor/internal/llm/groq"
	"github.com/kalke/personal-document-extractor/internal/migrate"
	"github.com/kalke/personal-document-extractor/internal/ratelimit"
	"github.com/kalke/personal-document-extractor/internal/store"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	setupLogger(cfg.LogLevel, cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := migrate.Up(ctx, cfg.DatabaseURL); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisCache := cache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.RedisCacheTTL)
	defer func() { _ = redisCache.Close() }()
	if err := redisCache.Ping(ctx); err != nil {
		slog.Warn("redis unavailable at startup; cache will fail open", "err", err)
	}

	llm := groq.New(cfg.GroqAPIKey, cfg.GroqModel, cfg.GroqBaseURL)
	svc := extract.NewService(
		llm,
		address_proof.DocType{},
		identity_document.DocType{},
		invoice_nf.DocType{},
	)

	users := store.NewUsers(pool)
	apiKeys := store.NewAPIKeys(pool)
	authenticator, err := auth.NewAuthenticator(auth.Options{
		Keys:     apiKeys,
		Users:    identity.Directory{Users: users},
		Domain:   cfg.Auth0Domain,
		Audience: cfg.Auth0Audience,
	})
	if err != nil {
		slog.Error("auth", "err", err)
		os.Exit(1)
	}
	limiter := ratelimit.New(redisCache.Redis(), cfg.RateLimitPerMinute)

	handler, err := httpapi.New(httpapi.Deps{
		Extractor:      svc,
		Pool:           pool,
		Cache:          redisCache,
		Extractions:    store.NewExtractions(pool),
		Users:          users,
		APIKeys:        apiKeys,
		TrustedProxies: cfg.TrustedProxies,
		Auth:           authenticator,
		RateLimit:      limiter,
		RequiredScope:  auth.ScopeExtractWrite,
	})
	if err != nil {
		slog.Error("httpapi", "err", err)
		os.Exit(1)
	}

	addr := ":" + cfg.Port
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      130 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", addr, "model", cfg.GroqModel)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
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
