package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/kalke/personal-document-extractor/internal/auth"
	"github.com/kalke/personal-document-extractor/internal/db"
	"github.com/kalke/personal-document-extractor/internal/store"
)

func main() {
	_ = godotenv.Load()

	fs := flag.NewFlagSet("apikey", flag.ExitOnError)
	name := fs.String("name", "local", "human-readable key name")
	scopes := fs.String("scopes", auth.ScopeExtractWrite, "comma-separated scopes")
	admin := fs.Bool("admin", false, "create a key with the admin scope (implies all permissions)")
	_ = fs.Parse(os.Args[1:])

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	scopeCSV := *scopes
	if *admin {
		scopeCSV = auth.ScopeAdmin
		if strings.TrimSpace(*name) == "local" {
			*name = "admin"
		}
	}
	parsedScopes, err := auth.ParseScopesCSV(scopeCSV)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scopes: %v\n", err)
		os.Exit(1)
	}

	plaintext, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	rec, err := store.NewAPIKeys(pool).Create(ctx, store.CreateAPIKeyInput{
		Name:      strings.TrimSpace(*name),
		KeyPrefix: prefix,
		KeyHash:   hash,
		Scopes:    parsedScopes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created API key %s (%s)\n", rec.ID, rec.Name)
	fmt.Printf("Prefix:  %s\n", rec.KeyPrefix)
	fmt.Printf("Scopes:  %s\n", strings.Join(rec.Scopes, ","))
	fmt.Println("Secret (store securely; shown once):")
	fmt.Println(plaintext)
}
