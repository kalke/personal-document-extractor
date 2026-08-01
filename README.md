# Personal Document Extractor

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-green.svg)](openapi/openapi.yaml)

Small Go API that turns Brazilian identity docs, address proofs, and invoices into structured JSON.

**API contract:** [`openapi/openapi.yaml`](openapi/openapi.yaml) (render with [Swagger Editor](https://editor.swagger.io/) or Redoc).  
Upload a PDF or image with a `doc_type`, get typed fields back. Extraction uses Groq vision; PDFs are rasterized locally with `pdftoppm`. Successful results are cached in Redis and stored in Postgres.

## Quick start (Docker)

Prerequisites: [Docker](https://docs.docker.com/get-docker/) with Compose v2, and [Make](https://www.gnu.org/software/make/).

Sibling IdP: [`kalke-auth`](https://github.com/kalke/kalke-auth) next to this repo (shared Docker network `kalke-auth`).

```bash
make setup                 # creates .env (+ kalke-auth/.env if sibling exists)
# edit .env → set GROQ_API_KEY

make setup-oidc            # OIDC_ISSUER / OIDC_AUDIENCE
make up-all                # Keycloak+Caddy + API+Postgres+Redis (all containers)
make ready
make smoke-m2m             # expect HTTP 400 (auth OK)
```

Everything is in Docker Desktop: `api`, `postgres`, `redis`, plus kalke-auth (`caddy`, `keycloak`, …).

- API: `http://localhost:8080`
- IdP (host): `http://localhost:8443`
- Inside Compose, the API reaches JWKS via `http://caddy:8443` (`OIDC_DISCOVERY_URL`) while JWT `iss` stays `http://localhost:8443/realms/kalke`.
- Production: **`pde.kalke.dev`** (Cloudflare Containers + Neon + Upstash) — see [DEPLOY.md](DEPLOY.md). Hosted sandbox: [kalke.dev](https://kalke.dev).

```bash
make reset                 # recreate API stack, truncate extractions
make logs SERVICE=api
make down / make down-all
make destroy               # wipe API volumes
```

Optional host API (`make deps-up && make run`) still works if `OIDC_DISCOVERY_URL` is unset.

### Everyday Make targets

| Target | What it does |
|---|---|
| `make help` | List all targets |
| `make setup` | Create `.env` if missing (+ sibling `kalke-auth/.env`) |
| `make setup-oidc` | Enable local OIDC_* pointing at kalke-auth |
| `make up` | Build/start API + Postgres + Redis (needs network `kalke-auth`) |
| `make up-all` | `auth-up` + `up` (full Docker) |
| `make deps-up` | Postgres + Redis only |
| `make up-fg` | API stack, foreground |
| `make down` / `make down-all` | Stop API stack / also stop kalke-auth |
| `make reset` | Recreate full API Compose stack + truncate `extractions` |
| `make destroy` | Stop and remove volumes |
| `make auth-up` / `auth-down` / `auth-logs` | Manage sibling [`kalke-auth`](../kalke-auth) |
| `make auth-jwks` / `auth-token` | JWKS check / demo access token |
| `make smoke-oidc` | Token → `POST /v1/extract` without file (expect 400) |
| `make logs` | Tail logs (`SERVICE=api` optional) |
| `make ps` | Compose status |
| `make build` | Build images |
| `make migrate` | Apply goose migrations **in Docker** |
| `make migrations NAME=…` | Create a new SQL migration file |
| `make health` / `make ready` | Hit `/health` and `/ready` |
| `make auth-token` / `auth-m2m-token` | Human / M2M JWT from kalke-auth |
| `make smoke-oidc` / `smoke-m2m` | Token → extract auth smoke (expect 400) |
| `make extract FILE=…` | `POST /v1/extract` (`TOKEN=` required) |
| `make lint` | golangci-lint (same as CI) |
| `make test` | Unit + integration tests (no Docker) |
| `make test-race` / `make test-cover` | Race detector / coverage |
| `make ci` | `lint` + `test` + build |
| `make run` | Run API on the host (optional) |

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
export TOKEN="$(make -s auth-m2m-token)"   # or: make -s auth-token

curl -X POST "http://localhost:8080/v1/extract?doc_type=identity_document" \
  -H "Authorization: Bearer ${TOKEN}" \
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

### Authentication and authorization

| Concern | Mechanism |
|---|---|
| **AuthN** (who) | `Authorization: Bearer` — OIDC JWT from [`kalke-auth`](../kalke-auth) (Keycloak) |
| **AuthZ** (what) | JWT `permissions` claim (realm roles) |

| Scope | Allows |
|---|---|
| `extract:write` | `POST /v1/extract` |
| `admin` | All scopes |

**Human:** password (dev `kalke-cli`) or Authorization Code + PKCE (`kalke-spa`).  
**M2M:** Keycloak `client_credentials` (`pde-m2m`) — no proprietary product API keys.

Identity lives in the IdP. This API does **not** keep a local `users` table. Successful extracts store JWT audit fields: `auth_subject` (`sub` UUID), `auth_client` (`azp`, e.g. `pde-m2m` / `kalke-cli`), and `auth_email` when the token includes it (humans).

`OIDC_ISSUER` + `OIDC_AUDIENCE` are **required**. JWKS comes from OIDC discovery. Empty `permissions` fail closed (no implicit write). `/health` and `/ready` stay public.

#### OIDC setup

1. Sibling [`kalke-auth`](../kalke-auth): `make auth-up` (re-import realm: `make -C ../kalke-auth destroy && make auth-up`).
2. `make setup-oidc`
3. `make up` / `make reset` / `make up-all` (API in Docker)
4. Human smoke: `make smoke-oidc` · M2M smoke: `make smoke-m2m`
5. IdP roles → `permissions`: `extract:write`, `admin`

The API container joins Docker network `kalke-auth` and loads JWKS from `http://caddy:8443` (`OIDC_DISCOVERY_URL`) while validating JWT `iss` as `http://localhost:8443/realms/kalke` (`OIDC_ISSUER`).

### Rate limiting

Authenticated extract calls are limited per principal (JWT `sub`) via Redis fixed 1-minute windows (`RATE_LIMIT_PER_MINUTE`, default **60**). Responses include `X-RateLimit-Limit` and `X-RateLimit-Remaining`. On exceed or Redis failure the API returns **429** (fail-closed) with `Retry-After`.

### `POST /v1/extract`

Requires Bearer auth + `extract:write` (or `admin`).

Query:

- `doc_type` (required)
- `refresh` (optional) — `true` / `1` / `yes` skips Redis, soft-deletes prior Postgres row for the same hash, re-extracts, and writes a new row

Body: `multipart/form-data` field `file`.

Example response (only `doc_type` + `data` — no `meta`):

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
  }
}
```

| Status | When | Typical `error` |
|---|---|---|
| `400` | Missing `doc_type` / file, bad media, MIME mismatch, preprocess | see [OpenAPI error catalog](openapi/openapi.yaml) |
| `401` | Missing/invalid Bearer or inactive user | `unauthorized` / `missing or invalid authorization` |
| `403` | Authenticated but missing scope | `forbidden` |
| `413` | Upload too large | `uploaded file exceeds size limit` |
| `422` | Model JSON could not be parsed | `could not process document` |
| `429` | Rate limit exceeded or Redis limiter down | `rate limit exceeded` / `rate limit unavailable` |
| `500` | Unexpected extract failure | `could not process document` |
| `502` | Groq request failed | `extraction provider unavailable` |

`GET /ready` returns **503** with `{status,db,redis}` when Postgres is down (not an `error` envelope).

Body shape for failures: `{"error":"…"}`. Full status/message catalog: [`openapi/openapi.yaml`](openapi/openapi.yaml) (`components.schemas.ErrorCatalog` + per-response examples).

### Output conventions

- **Dates** — ISO `YYYY-MM-DD`, or `null`
- **CPF / CNPJ / CEP** — digits only
- **`numero_documento`** — RG when `tipo=RG`, CNH registry when `tipo=CNH`

## Cache & persistence

- **Key:** `extract:v1:{doc_type}:{sha256(raw bytes)}`
- **TTL:** `REDIS_CACHE_TTL` (default `24h`, must be `> 0`)
- **Order:** validate/prepare upload, then cache lookup (spoofed extensions cannot skip validation)
- **Redis fail-open:** warn and skip cache if Redis is down; corrupt cache entries are deleted. Cache is written only after a successful Postgres persist (so a DB failure cannot poison Redis into permanent cache-hits with no rows).
- **Hit:** return Redis payload only (no Postgres write)
- **Miss:** extract → Redis SETEX → Postgres INSERT; PG write failure after success still returns `200`
- **`refresh=true`:** delete Redis key, soft-delete active Postgres row (`deleted_at`), re-extract, insert new row + cache
- **Request origin:** on persist, store `client_ip` (trusted-proxy aware) and `user_agent` in Postgres only — never returned in the API response
- **Principal:** on persist, store `auth_subject`, `auth_client` (`azp`), and `auth_email` when present

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
| `RATE_LIMIT_PER_MINUTE` | `60` | per authenticated principal; must be `> 0` |
| `OIDC_ISSUER` | — | **required** OIDC issuer URL |
| `OIDC_AUDIENCE` | — | **required** API audience |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `text` | Compose forces `json` |
| `TRUSTED_PROXIES` | empty | CIDRs/IPs that may set `X-Forwarded-For` / `X-Real-IP`; empty = use `RemoteAddr` only |
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
- **Integration tests** exercise `/health`, `/ready`, and `/v1/extract` with fakes + [miniredis](https://github.com/alicebob/miniredis). Store tests run against real Postgres when `DATABASE_URL` is set (CI).

GitHub Actions runs **lint**, **goose migrate** against a Postgres 18 service, **`make test`** (including store integration tests when `DATABASE_URL` is set), binary builds, then **`docker build`**. Locally, `make ci` covers lint + test + build without Docker; set `DATABASE_URL` to exercise Postgres integration tests.

## Layout

```
LICENSE                 Apache-2.0
openapi/openapi.yaml    Public API contract
Makefile                Docker-first commands
docker-compose.yml      api + postgres + redis
Dockerfile              api + migrate binaries
migrations/             goose SQL (embedded)
cmd/api                 HTTP entrypoint (migrates on boot)
cmd/migrate             goose up CLI
internal/config         Env
internal/db             pgx pool
internal/auth           OIDC JWT (JWKS discovery)
internal/authz          Scope checks
internal/ratelimit      Redis per-principal limiter
internal/cache          Redis extract cache
internal/store          extractions
internal/httpapi        /health, /ready, /v1/*
internal/preprocess     validation, PDF render, image compact
internal/normalize      CPF/CNPJ/CEP/date helpers
internal/llm/groq       Groq client
internal/extract        Prompt → LLM → decode → normalize
internal/doctypes/*     Per-type schema + prompts
```

## Roadmap (intentionally out of v1)

- Website / OIDC login UI (Next.js SPA + kalke-auth)
- Social IdP connections in Keycloak
- Web3 wallet linking (`user_identities` when needed)
- Fine-grained AuthZ (OPA / SpiceDB)
- Multi-tenant orgs / billing

## Adding a document type

1. Add `internal/doctypes/<name>/` implementing `extract.DocType`
2. Register it in `cmd/api/main.go`
