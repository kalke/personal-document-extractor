# Personal Document Extractor

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-green.svg)](openapi/openapi.yaml)

Small Go API that turns Brazilian identity docs, address proofs, and invoices into structured JSON.

**API contract:** [`openapi/openapi.yaml`](openapi/openapi.yaml) (render with [Swagger Editor](https://editor.swagger.io/) or Redoc).  
**Arquitetura (PT-BR):** [`docs/COMO-FUNCIONA.md`](docs/COMO-FUNCIONA.md) — como funciona, decisões, auth, usuários e M2M.

Upload a PDF or image with a `doc_type`, get typed fields back. Extraction uses Groq vision; PDFs are rasterized locally with `pdftoppm`. Successful results are cached in Redis and stored in Postgres.

## Quick start (Docker)

Prerequisites: [Docker](https://docs.docker.com/get-docker/) with Compose v2, and [Make](https://www.gnu.org/software/make/).

Sibling IdP (optional, for JWT): clone/create [`kalke-auth`](../kalke-auth) next to this repo.

```bash
make setup                 # creates .env (+ kalke-auth/.env if sibling exists)
# edit .env → set GROQ_API_KEY

make up                    # build + start api, postgres, redis
make ready                 # wait until DB is up: {"status":"ready",...}
make apikey NAME=local     # create extract:write API key (secret once)
make admin                 # create admin API key (scope=admin; secret once)
```

API listens on `http://localhost:8080`. `/v1/extract` requires `Authorization: Bearer <api-key-or-jwt>`.

### Full local stack (API + OIDC IdP)

```bash
make setup
# set GROQ_API_KEY in .env

make up-all                # kalke-auth (Caddy:8443) + this Compose stack
make auth-jwks             # sanity: JWKS via public proxy
make apikey NAME=local     # M2M still works without OIDC env
```

### JWT smoke (host API + kalke-auth)

Compose API containers cannot reach `localhost:8443` on the host. For OIDC end-to-end:

```bash
make setup-oidc            # writes OIDC_ISSUER / OIDC_AUDIENCE into .env
make auth-up
make deps-up               # postgres + redis only
make run                   # API on host (separate terminal)
make smoke-oidc            # demo token → GET /v1/me
# or: make me TOKEN=$(make -s auth-token)
```

```bash
make logs                  # follow all logs
make logs SERVICE=api      # API only

make down                  # stop API stack (keeps DB volume)
make down-all              # stop API stack + kalke-auth
make destroy               # stop and delete volumes
```

### Everyday Make targets

| Target | What it does |
|---|---|
| `make help` | List all targets |
| `make setup` | Create `.env` if missing (+ sibling `kalke-auth/.env`) |
| `make setup-oidc` | Enable local OIDC_* pointing at kalke-auth |
| `make up` | Build and start the API stack (detached) |
| `make up-all` | `auth-up` + `up` |
| `make deps-up` | Postgres + Redis only (for host `make run`) |
| `make up-fg` | API stack, foreground |
| `make down` / `make down-all` | Stop API stack / also stop kalke-auth |
| `make destroy` | Stop and remove volumes |
| `make auth-up` / `auth-down` / `auth-logs` | Manage sibling [`kalke-auth`](../kalke-auth) |
| `make auth-jwks` / `auth-token` | JWKS check / demo access token |
| `make smoke-oidc` | Token → `GET /v1/me` |
| `make logs` | Tail logs (`SERVICE=api` optional) |
| `make ps` | Compose status |
| `make build` | Build images |
| `make migrate` | Apply goose migrations **in Docker** |
| `make migrations NAME=…` | Create a new SQL migration file |
| `make apikey NAME=…` | Create a hashed API key (`SCOPES=` optional) |
| `make admin` | Create an admin API key (`NAME=` optional, default `admin`) |
| `make health` / `make ready` | Hit `/health` and `/ready` |
| `make me` | `GET /v1/me` (`API_KEY=` or `TOKEN=`) |
| `make extract FILE=…` | `POST /v1/extract` (`API_KEY=` or `TOKEN=`) |
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
export API_KEY='pde_live_…'   # from make apikey

curl -X POST "http://localhost:8080/v1/extract?doc_type=identity_document" \
  -H "Authorization: Bearer ${API_KEY}" \
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
| **AuthN** (who) | `Authorization: Bearer` — API key (`pde_live_…`) and/or OIDC JWT |
| **AuthZ** (what) | Scopes on the key / JWT `permissions` claim |

| Scope | Allows |
|---|---|
| `extract:write` | `POST /v1/extract` |
| `keys:manage` | `GET/POST/DELETE /v1/api-keys` (own keys) |
| `admin` | All scopes |

**Users:** Human login lives in an external **OIDC IdP** (local default: sibling repo [`kalke-auth`](../kalke-auth) — Keycloak behind Caddy). This API keeps a local `users` row keyed by OIDC `sub` (`auth_subject`) and owns `api_keys.user_id`. There is **no** password stored in Postgres.

On JWT auth the API upserts `users` and exposes:

| Method | Path | Notes |
|---|---|---|
| `GET` | `/v1/me` | Current user profile |
| `GET` | `/v1/api-keys` | List own keys (prefix only) |
| `POST` | `/v1/api-keys` | Create key for self (default scope `extract:write`; secret once) |
| `DELETE` | `/v1/api-keys/{id}` | Revoke own key |

API keys are **SHA-256 hashed** with a public prefix. Bootstrap ops keys: `make apikey` / `make admin` (attached to `system:ops`). Set `OIDC_ISSUER` + `OIDC_AUDIENCE` for JWT. JWKS is resolved via OIDC discovery (`/.well-known/openid-configuration`). If the token has no `permissions`, the API grants default `extract:write` + `keys:manage`. `/health` and `/ready` stay public.

#### OIDC setup (website-ready)

1. Ensure sibling [`kalke-auth`](../kalke-auth) exists, then `make auth-up` (or `make up-all`).
2. `make setup-oidc` (or set manually):
   - `OIDC_ISSUER=http://localhost:8443/realms/kalke`
   - `OIDC_AUDIENCE=personal-document-extractor`
3. SPA: Authorization Code + PKCE (`kalke-spa` client); send access token as `Authorization: Bearer`.
4. Dev smoke: `make smoke-oidc` (host API) or `make me TOKEN=$(make -s auth-token)`.
5. Optional IdP roles → claim `permissions`: `extract:write`, `keys:manage`, `admin`.

When the API runs **inside Docker**, `localhost` is the container — prefer host `make run` for JWT smoke, or align `KC_HOSTNAME` / `OIDC_ISSUER` to a host-reachable name.

### Rate limiting

Authenticated extract calls are limited per principal (API key id / JWT `sub`) via Redis fixed 1-minute windows (`RATE_LIMIT_PER_MINUTE`, default **60**). Responses include `X-RateLimit-Limit` and `X-RateLimit-Remaining`. On exceed or Redis failure the API returns **429** (fail-closed) with `Retry-After`.

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
- **Redis fail-open:** warn and skip cache if Redis is down; corrupt cache entries are deleted
- **Hit:** return Redis payload only (no Postgres write)
- **Miss:** extract → Redis SETEX → Postgres INSERT; PG write failure after success still returns `200`
- **`refresh=true`:** delete Redis key, soft-delete active Postgres row (`deleted_at`), re-extract, insert new row + cache
- **Request origin:** on persist, store `client_ip` (trusted-proxy aware) and `user_agent` in Postgres only — never returned in the API response
- **Principal:** on persist, store `api_key_id` / `auth_subject` for audit

## Configuration

Copy from [`.env.example`](.env.example) via `make setup`:

| Env | Default | Notes |
|---|---|---|
| `GROQ_API_KEY` | — | **required** |
| `GROQ_MODEL` | `qwen/qwen3.6-27b` | vision model |
| `PORT` | `8080` | host publish port for Compose |
| `DATABASE_URL` | example | required for `make run` / `make apikey`; Compose overrides with internal URL |
| `REDIS_ADDR` | `localhost:6379` | for `make run`; Compose uses `redis:6379` |
| `REDIS_CACHE_TTL` | `24h` | Go duration; must be `> 0` |
| `RATE_LIMIT_PER_MINUTE` | `60` | per authenticated principal; must be `> 0` |
| `OIDC_ISSUER` | empty | OIDC issuer URL (with `OIDC_AUDIENCE`) |
| `OIDC_AUDIENCE` | empty | API audience for JWT validation |
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
- **Integration tests** exercise `/health`, `/ready`, `/v1/extract`, `/v1/me`, and `/v1/api-keys` with fakes + [miniredis](https://github.com/alicebob/miniredis). Store tests run against real Postgres when `DATABASE_URL` is set (CI).

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
cmd/apikey              Create API keys (secret once)
internal/config         Env
internal/db             pgx pool
internal/auth           API keys + OIDC JWT
internal/identity       JWT → local users upsert
internal/authz          Scope checks
internal/ratelimit      Redis per-principal limiter
internal/cache          Redis extract cache
internal/store          users, api_keys, extractions
internal/httpapi        /health, /ready, /v1/*
internal/preprocess     validation, PDF render, image compact
internal/normalize      CPF/CNPJ/CEP/date helpers
internal/llm/groq       Groq client
internal/extract        Prompt → LLM → decode → normalize
internal/doctypes/*     Per-type schema + prompts
```

## Roadmap (intentionally out of v1)

- Website / OIDC login UI (Next.js SPA + kalke-auth / any IdP)
- Apple / Google / GitHub social IdP connections
- Web3 wallet linking (`user_identities` when needed)
- Fine-grained AuthZ (OPA / SpiceDB)
- Multi-tenant orgs / billing

## Adding a document type

1. Add `internal/doctypes/<name>/` implementing `extract.DocType`
2. Register it in `cmd/api/main.go`
