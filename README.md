# Personal Document Extractor

Small Go API that turns Brazilian identity docs, address proofs, and invoices into structured JSON.

Upload a PDF or image with a `doc_type`, get typed fields back. Extraction uses Groq vision; PDFs are rasterized locally with `pdftoppm`. Successful results are cached in Redis and stored in Postgres.

## Quick start (Docker)

Prerequisites: [Docker](https://docs.docker.com/get-docker/) with Compose v2, and [Make](https://www.gnu.org/software/make/).

```bash
make setup                 # creates .env from .env.example
# edit .env → set GROQ_API_KEY

make up                    # build + start api, postgres, redis
make ready                 # wait until DB is up: {"status":"ready",...}
```

API listens on `http://localhost:8080`.

```bash
make logs                  # follow all logs
make logs SERVICE=api      # API only

make down                  # stop (keeps DB volume)
make destroy               # stop and delete volumes
```

### Everyday Make targets

| Target | What it does |
|---|---|
| `make help` | List all targets |
| `make setup` | Create `.env` if missing |
| `make up` | Build and start the stack (detached) |
| `make up-fg` | Same, foreground |
| `make down` | Stop containers |
| `make destroy` | Stop and remove volumes |
| `make logs` | Tail logs (`SERVICE=api` optional) |
| `make ps` | Compose status |
| `make build` | Build images |
| `make migrate` | Apply goose migrations **in Docker** |
| `make migrations NAME=…` | Create a new SQL migration file |
| `make health` / `make ready` | Hit `/health` and `/ready` |
| `make lint` | golangci-lint (same as CI) |
| `make test` | Unit + integration tests (no Docker) |
| `make test-race` / `make test-cover` | Race detector / coverage |
| `make ci` | `lint` + `test` + build |
| `make run` | Run API on the host (optional local path) |

## Migrations

SQL files live in [`migrations/`](migrations/) and are embedded into the binaries (goose). The API also applies pending migrations on startup.

**Create a new migration** (sequential goose SQL under `migrations/`):

```bash
make migrations NAME=add_indexes
# edit migrations/0000X_add_indexes.sql
```

Uses the goose Docker image when Docker is available; otherwise falls back to `go run` (same goose version as the module).

**Apply migrations** (Docker; starts Postgres if needed, runs `/app/migrate` in the API image):

```bash
make migrate
```

You normally do **not** need `make migrate` after `make up` — the API migrates on boot. Use it when you want to apply SQL without restarting the API, or as an explicit ops step.

## Extract something

```bash
curl -X POST "http://localhost:8080/v1/extract?doc_type=identity_document" \
  -F "file=@./documento.pdf"
```

Supported `doc_type` values:

| `doc_type` | What it extracts |
|---|---|
| `identity_document` | RG / CNH (including CNH-e) |
| `address_proof` | Comprovante de endereço |
| `invoice_nf` | NFe / NFSe (simplified) |

Uploads: PDF, JPEG, PNG, WebP (max **32 MiB**). MIME is sniffed from bytes; spoofed headers and extension mismatches are rejected.

## API

### `GET /health`

Liveness (no dependency checks):

```bash
make health
# or: curl http://localhost:8080/health
```

### `GET /ready`

Postgres required (`503` if down). Redis is best-effort informational only:

```bash
make ready
```

### `POST /v1/extract`

Query:

- `doc_type` (required)
- `refresh` (optional) — `true` / `1` / `yes` skips Redis, soft-deletes prior Postgres row for the same hash, re-extracts, and writes a new row

Body: `multipart/form-data` field `file`.

Example response:

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
    "filename": "documento.pdf",
    "content_sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "cache": "miss"
  }
}
```

`meta.cache` is `hit` or `miss`. `meta.content_sha256` is SHA-256 of the **raw upload bytes**.

| Status | When | Typical `error` |
|---|---|---|
| `400` | Missing `doc_type` / file, bad media, MIME mismatch | validation message / `unknown doc_type` |
| `413` | Upload too large | `uploaded file exceeds size limit` |
| `422` | Model JSON could not be parsed | `could not process document` |
| `502` | Groq request failed | `extraction provider unavailable` |
| `503` | `/ready` when Postgres is down | — |

Body shape: `{"error":"…"}`. Provider internals are logged server-side, not returned.

### Output conventions

- **Dates** — ISO `YYYY-MM-DD`, or `null`
- **CPF / CNPJ / CEP** — digits only
- **`numero_documento`** — RG when `tipo=RG`, CNH registry when `tipo=CNH`

## Cache & persistence

- **Key:** `extract:v1:{doc_type}:{sha256(raw bytes)}`
- **TTL:** `REDIS_CACHE_TTL` (default `24h`, must be `> 0`)
- **Order:** validate/prepare upload, then cache lookup (spoofed extensions cannot skip validation)
- **Redis fail-open:** warn and skip cache if Redis is down; corrupt cache entries are deleted
- **Hit:** return Redis payload only (no Postgres write)
- **Miss:** extract → Redis SETEX → Postgres INSERT; PG write failure after success still returns `200`
- **`refresh=true`:** delete Redis key, soft-delete active Postgres row (`deleted_at`), re-extract, insert new row + cache

## Configuration

Copy from [`.env.example`](.env.example) via `make setup`:

| Env | Default | Notes |
|---|---|---|
| `GROQ_API_KEY` | — | **required** |
| `GROQ_MODEL` | `qwen/qwen3.6-27b` | vision model |
| `PORT` | `8080` | host publish port for Compose |
| `DATABASE_URL` | example | required for `make run`; Compose overrides with internal URL |
| `REDIS_ADDR` | `localhost:6379` | for `make run`; Compose uses `redis:6379` |
| `REDIS_CACHE_TTL` | `24h` | Go duration; must be `> 0` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `text` | Compose forces `json` |
| `POSTGRES_USER` / `PASSWORD` / `DB` | `extractor` | Compose Postgres |

Postgres/Redis Compose ports bind to `127.0.0.1` only.

## Local development (optional)

If you prefer running the API on the host (Compose still useful for Postgres/Redis):

```bash
make setup
make up                    # or only: docker compose up -d postgres redis
# install poppler for PDFs:
#   sudo apt-get install -y poppler-utils

make run                   # go run ./cmd/api
make migrate-local         # goose up without Docker
```

Dev reload with [Air](https://github.com/air-verse/air):

```bash
go install github.com/air-verse/air@latest
air
```

### Quality

```bash
make lint          # golangci-lint
make test          # unit + HTTP/cache integration tests
make test-race     # optional
make ci            # lint + test + binary build
```

- **Unit tests** cover config, normalize, doctype `Normalize`, extract JSON decoding, cache keys, and fail-open nil paths.
- **Integration tests** exercise `/health`, `/ready`, and `/v1/extract` with fakes + [miniredis](https://github.com/alicebob/miniredis) (validate-before-cache, hit/miss, stable errors, persistence). No live Groq/Postgres required.

GitHub Actions also runs `docker build`; `make ci` is the local gate without requiring Docker.

## Layout

```
Makefile                Docker-first commands
docker-compose.yml      api + postgres + redis
Dockerfile              api + migrate binaries
migrations/             goose SQL (embedded)
cmd/api                 HTTP entrypoint (migrates on boot)
cmd/migrate             goose up CLI
internal/config         Env
internal/db             pgx pool
internal/cache          Redis
internal/store          Postgres extractions
internal/httpapi        /health, /ready, /v1/extract
internal/preprocess     validation, PDF render, image compact
internal/normalize      CPF/CNPJ/CEP/date helpers
internal/llm/groq       Groq client
internal/extract        Prompt → LLM → decode → normalize
internal/doctypes/*     Per-type schema + prompts
```

## Adding a document type

1. Add `internal/doctypes/<name>/` implementing `extract.DocType`
2. Register it in `cmd/api/main.go`
