# Personal Document Extractor

Go HTTP API that extracts structured JSON from Brazilian PDFs using a Groq LLM.

Same *idea* as a document-parser (upload → LLM → typed JSON), built as a new personal service — not a port of any work codebase.

## Supported `doc_type` values

| `doc_type` | Document |
|------------|----------|
| `address_proof` | Comprovante de endereço |
| `identity_document` | RG / CNH |
| `invoice_nf` | Nota fiscal (NFe/NFSe simplificada) |

## Prerequisites

- Go 1.22+
- A free [Groq API key](https://console.groq.com)

## Setup

```bash
cp .env.example .env
# edit .env and set GROQ_API_KEY
```

```bash
go mod tidy
go run ./cmd/api
```

Server listens on `http://localhost:8080` by default.

## API

### Health

```bash
curl http://localhost:8080/health
```

### Extract

```bash
curl -X POST "http://localhost:8080/v1/extract?doc_type=address_proof" \
  -F "file=@./sample.pdf"
```

Response shape:

```json
{
  "doc_type": "address_proof",
  "data": { "...": "typed fields" },
  "meta": { "model": "llama-3.3-70b-versatile", "chars": 1234 }
}
```

### Errors

| Status | Meaning |
|--------|---------|
| 400 | Bad `doc_type`, missing file, non-PDF, or no extractable text |
| 422 | LLM returned invalid JSON after one repair attempt |
| 502 | Groq request failed |

## Architecture

```
cmd/api                 HTTP entrypoint
internal/config         Env config
internal/httpapi        chi routes
internal/preprocess     PDF → plain text
internal/llm/groq       OpenAI-compatible Groq client
internal/extract        Prompt → LLM → unmarshal (+ repair)
internal/doctypes/*     Per-type prompt + Go schema
```

MVP extracts **text from PDFs** then sends it to Groq. Scanned image-only PDFs need OCR (out of scope for v1).

## Add a document type

1. Create `internal/doctypes/<name>/`
2. Implement `extract.DocType` (`Name`, `SystemPrompt`, `SchemaHint`, `EmptyResult`)
3. Register it in `cmd/api/main.go`
