# Personal Document Extractor

Small Go API that turns Brazilian identity docs, address proofs, and invoices into structured JSON.

You upload a PDF or image, pick a `doc_type`, and get back typed fields. Under the hood it uses Groq’s vision model. PDFs are rasterized locally first — Groq’s chat vision API accepts images, not raw PDFs.

## Supported document types

| `doc_type` | What it extracts |
|---|---|
| `identity_document` | RG / CNH (including CNH-e) |
| `address_proof` | Comprovante de endereço |
| `invoice_nf` | NFe / NFSe (simplified) |

Unknown `doc_type` values are rejected.

## Supported uploads

| Type | Notes |
|---|---|
| `application/pdf` | Rendered with `pdftoppm`, then sent to vision |
| `image/jpeg` | Sent to vision directly |
| `image/png` | Sent to vision directly |
| `image/webp` | Sent to vision directly |

Validation sniffs file bytes (magic). Spoofed `Content-Type` headers are ignored. Extension must not contradict the sniffed type (e.g. a JPEG named `.pdf` is rejected). Anything else returns `400`.

Max upload size: **32 MiB**.

## Prerequisites

- Go 1.22+
- [`poppler-utils`](https://poppler.freedesktop.org/) (`pdftoppm`) for PDF rendering
- A [Groq API key](https://console.groq.com) with access to a vision model

On Ubuntu/WSL:

```bash
sudo apt-get install -y poppler-utils
```

## Setup

```bash
cp .env.example .env
# set GROQ_API_KEY
```

```bash
go mod tidy
go run ./cmd/api
```

Dev with live reload ([Air](https://github.com/air-verse/air)):

```bash
go install github.com/air-verse/air@latest
air
```

Listens on `:8080` by default (`PORT` in `.env`).

Default model: `qwen/qwen3.6-27b` (override with `GROQ_MODEL`).

Logging:

| Env | Default | Notes |
|---|---|---|
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `text` | use `json` in containers |

## Docker

```bash
docker build -t personal-document-extractor .
docker run --rm -p 8080:8080 --env-file .env personal-document-extractor
```

Image runs as non-root, includes `poppler-utils`, and exposes `/health` for the container healthcheck.

## API

### `GET /health`

```bash
curl http://localhost:8080/health
```

### `POST /v1/extract`

Query params:

- `doc_type` (required) — one of the values above

Body: `multipart/form-data` with field `file`.

```bash
curl -X POST "http://localhost:8080/v1/extract?doc_type=identity_document" \
  -F "file=@./documento.pdf"

curl -X POST "http://localhost:8080/v1/extract?doc_type=identity_document" \
  -F "file=@./documento.jpg"
```

Example response (`identity_document`):

```json
{
  "doc_type": "identity_document",
  "data": {
    "tipo": "CNH",
    "nome": "FULANO DA SILVA",
    "cpf": "52998224725",
    "numero_documento": "12345678901",
    "data_nascimento": "1990-01-15",
    "orgao_emissor": "DETRAN/SP",
    "validade": "2030-12-31"
  },
  "meta": {
    "model": "qwen/qwen3.6-27b",
    "chars": 1200,
    "mode": "vision",
    "images": 1,
    "filename": "documento.pdf"
  }
}
```

### Output conventions

- **Dates** — ISO `YYYY-MM-DD`, or `null` if missing/unparseable
- **CPF / CNPJ** — digits only (no mask)
- **CEP** — digits only
- **`numero_documento`** (identity) — RG number when `tipo=RG`, CNH registry number when `tipo=CNH`. Not both; we don’t invent the other ID.

Normalization runs server-side after the LLM response, so formatting stays stable even when the model returns BR-style dates or masked CPF.

### Errors

| Status | When |
|---|---|
| `400` | Missing `doc_type` / file, unsupported media, MIME/extension mismatch, preprocess failure |
| `422` | Model returned JSON we couldn’t parse (after one repair attempt) |
| `502` | Groq request failed |

Body shape: `{"error":"…"}`.

## Layout

```
cmd/api                 HTTP entrypoint
internal/config         Env
internal/httpapi        Routes
internal/preprocess     Upload validation, PDF render, image compact
internal/normalize      CPF/CNPJ/CEP/date helpers
internal/llm/groq       Groq client (text + vision)
internal/extract        Prompt → LLM → decode → normalize
internal/doctypes/*     Per-type schema + prompts
```

## Adding a document type

1. Add `internal/doctypes/<name>/` implementing `extract.DocType` (`Name`, `SystemPrompt`, `SchemaHint`, `EmptyResult`, `Normalize`)
2. Register it in `cmd/api/main.go`
