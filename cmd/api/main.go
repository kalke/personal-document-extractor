package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

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
		log.Fatalf("config: %v", err)
	}

	llm := groq.New(cfg.GroqAPIKey, cfg.GroqModel, cfg.GroqBaseURL)
	svc := extract.NewService(
		llm,
		address_proof.DocType{},
		identity_document.DocType{},
		invoice_nf.DocType{},
	)

	handler := httpapi.New(svc)
	addr := ":" + cfg.Port
	fmt.Fprintf(os.Stderr, "personal-document-extractor listening on %s (model=%s)\n", addr, cfg.GroqModel)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server: %v", err)
	}
}
