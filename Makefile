SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

.DEFAULT_GOAL := help

COMPOSE       ?= docker compose
GOOSE_IMAGE   ?= ghcr.io/pressly/goose:v3.27.0
GOOSE_PKG     ?= github.com/pressly/goose/v3/cmd/goose@v3.27.3
MIGRATIONS    := migrations
APP_PORT      ?= 8080
KALKE_AUTH_DIR ?= ../kalke-auth
OIDC_ISSUER_DEFAULT   ?= http://localhost:8443/realms/kalke
OIDC_AUDIENCE_DEFAULT ?= personal-document-extractor

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: setup
setup: ## Create .env from .env.example if missing; ensure sibling kalke-auth .env
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env — set GROQ_API_KEY before starting."; \
	else \
		echo ".env already exists"; \
	fi
	@if [ -d "$(KALKE_AUTH_DIR)" ]; then \
		if [ ! -f "$(KALKE_AUTH_DIR)/.env" ] && [ -f "$(KALKE_AUTH_DIR)/.env.example" ]; then \
			cp "$(KALKE_AUTH_DIR)/.env.example" "$(KALKE_AUTH_DIR)/.env"; \
			echo "Created $(KALKE_AUTH_DIR)/.env"; \
		fi; \
	else \
		echo "Note: $(KALKE_AUTH_DIR) not found. OIDC IdP (kalke-auth) is required — place it beside this repo."; \
	fi

.PHONY: setup-oidc
setup-oidc: setup ## Write local OIDC_* into .env for host JWT smoke (kalke-auth)
	@grep -vE '^(OIDC_ISSUER|OIDC_AUDIENCE)=' .env > .env.tmp || true
	@printf 'OIDC_ISSUER=%s\nOIDC_AUDIENCE=%s\n' \
		"$(OIDC_ISSUER_DEFAULT)" "$(OIDC_AUDIENCE_DEFAULT)" >> .env.tmp
	@mv .env.tmp .env
	@echo "Wrote OIDC_ISSUER=$(OIDC_ISSUER_DEFAULT)"
	@echo "Wrote OIDC_AUDIENCE=$(OIDC_AUDIENCE_DEFAULT)"
	@echo "JWT smoke: make auth-up && make deps-up && make run   then   make smoke-oidc"

.PHONY: check-env
check-env: ## Fail if GROQ_API_KEY is unset in the environment or .env
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	set +a; \
	if [ -z "$${GROQ_API_KEY:-}" ] || [ "$${GROQ_API_KEY}" = "your_groq_api_key_here" ]; then \
		echo "GROQ_API_KEY is required. Run: make setup && edit .env"; \
		exit 1; \
	fi

.PHONY: up
up: check-env ## Build and start API + Postgres + Redis (detached)
	$(COMPOSE) up --build -d

.PHONY: deps-up
deps-up: ## Start only Postgres + Redis (for host `make run` / JWT smoke)
	$(COMPOSE) up -d postgres redis

.PHONY: up-all
up-all: auth-up up ## Start kalke-auth IdP + this API stack

.PHONY: up-fg
up-fg: check-env ## Build and start stack in foreground
	$(COMPOSE) up --build

.PHONY: down
down: ## Stop stack (keeps volumes)
	$(COMPOSE) down

.PHONY: down-all
down-all: down auth-down ## Stop API stack and kalke-auth

.PHONY: destroy
destroy: ## Stop stack and delete volumes (destructive)
	$(COMPOSE) down -v

.PHONY: reset
reset: check-env ## Docker down + up (postgres/redis); free :8080; truncate; start host API
	@echo "Stopping listeners on :$(APP_PORT)..."
	@-fuser -k "$(APP_PORT)/tcp" >/dev/null 2>&1 || true
	@if [ -f .tmp/api.pid ]; then kill $$(cat .tmp/api.pid) >/dev/null 2>&1 || true; fi
	$(COMPOSE) down
	$(COMPOSE) up -d --wait postgres redis
	@set -a; [ -f .env ] && . ./.env; set +a; \
		user=$${POSTGRES_USER:-extractor}; \
		db=$${POSTGRES_DB:-extractor}; \
		$(COMPOSE) exec -T postgres psql -U "$$user" -d "$$db" -c "TRUNCATE TABLE extractions;"
	@mkdir -p bin .tmp
	@echo "Building and starting host API on :$(APP_PORT)..."
	@go build -o bin/api ./cmd/api
	@set -a; [ -f .env ] && . ./.env; set +a; \
		setsid -f ./bin/api >.tmp/api.log 2>&1; \
		sleep 0.3; \
		pgrep -n -x api >.tmp/api.pid || pgrep -n -f '[.]?/bin/api' >.tmp/api.pid
	@ok=0; for i in $$(seq 1 30); do \
		if curl -sf "http://localhost:$(APP_PORT)/health" >/dev/null 2>&1; then ok=1; break; fi; \
		sleep 1; \
	done; \
	if [ "$$ok" -eq 1 ]; then \
		echo "Reset OK — deps up, extractions truncated, API pid=$$(cat .tmp/api.pid) on :$(APP_PORT)"; \
		echo "Logs: tail -f .tmp/api.log   | stop: kill \$$(cat .tmp/api.pid)"; \
	else \
		echo "API failed to become ready — see .tmp/api.log"; \
		tail -n 40 .tmp/api.log || true; \
		exit 1; \
	fi

.PHONY: stop-api
stop-api: ## Stop host API started by make reset
	@-fuser -k "$(APP_PORT)/tcp" >/dev/null 2>&1 || true
	@if [ -f .tmp/api.pid ]; then kill $$(cat .tmp/api.pid) >/dev/null 2>&1 || true; rm -f .tmp/api.pid; fi
	@echo "Host API stopped"

.PHONY: restart
restart: ## Restart all Compose services
	$(COMPOSE) restart

.PHONY: ps
ps: ## Show Compose service status
	$(COMPOSE) ps

.PHONY: logs
logs: ## Tail Compose logs (optional: SERVICE=api)
	$(COMPOSE) logs -f --tail=200 $(SERVICE)

.PHONY: build
build: ## Build Compose images
	$(COMPOSE) build

.PHONY: migrate
migrate: ## Apply goose migrations via Docker (Postgres must be reachable)
	$(COMPOSE) up -d postgres
	$(COMPOSE) run --rm --no-deps --entrypoint /app/migrate api

.PHONY: migrations
migrations: ## Create a new SQL migration: make migrations NAME=add_foo
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrations NAME=add_something"; \
		exit 1; \
	fi
	@if docker info >/dev/null 2>&1; then \
		docker run --rm \
			--user "$(shell id -u):$(shell id -g)" \
			-v "$(CURDIR)/$(MIGRATIONS):/migrations" \
			$(GOOSE_IMAGE) \
			-s -dir /migrations create $(NAME) sql; \
	else \
		echo "docker unavailable; creating migration with go run ($(GOOSE_PKG))"; \
		go run $(GOOSE_PKG) -s -dir $(MIGRATIONS) create $(NAME) sql; \
	fi
	@echo "Created migration under $(MIGRATIONS)/ — fill in +goose Up/Down, then: make migrate"

.PHONY: lint
lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null || { \
		echo "golangci-lint not found. Install: https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	}
	golangci-lint run ./...

.PHONY: test
test: ## Run unit + integration tests (stdlib + miniredis; no Docker required)
	go test ./... -count=1

.PHONY: test-race
test-race: ## Run tests with the race detector
	go test ./... -race -count=1

.PHONY: test-cover
test-cover: ## Run tests with coverage summary
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -n 1

.PHONY: fmt
fmt: ## Format Go sources with gofmt
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	go mod tidy

.PHONY: ci
ci: lint test ## Lint + tests + binary build (GitHub Actions also runs docker build)
	go build -o /tmp/personal-document-extractor-api ./cmd/api
	go build -o /tmp/personal-document-extractor-migrate ./cmd/migrate

.PHONY: run
run: check-env ## Run API on the host (needs local Go, Postgres, Redis, poppler, OIDC_*)
	go run ./cmd/api

.PHONY: migrate-local
migrate-local: ## Apply migrations with local Go (uses DATABASE_URL from .env)
	go run ./cmd/migrate

.PHONY: health
health: ## Curl /health
	@curl -sS "http://localhost:$(APP_PORT)/health"; echo

.PHONY: ready
ready: ## Curl /ready
	@curl -sS "http://localhost:$(APP_PORT)/ready"; echo

.PHONY: extract
extract: ## POST /v1/extract (TOKEN= FILE=path [DOC_TYPE=identity_document])
	@if [ -z "$(TOKEN)" ] || [ -z "$(FILE)" ]; then \
		echo "Usage: make extract TOKEN=<jwt> FILE=./doc.pdf [DOC_TYPE=identity_document]"; \
		exit 1; \
	fi
	@curl -sS -X POST \
		"http://localhost:$(APP_PORT)/v1/extract?doc_type=$(or $(DOC_TYPE),identity_document)" \
		-H "Authorization: Bearer $(TOKEN)" \
		-F "file=@$(FILE)"; echo

.PHONY: auth-up
auth-up: ## Start sibling kalke-auth (OIDC IdP behind Caddy)
	@test -d "$(KALKE_AUTH_DIR)" || { echo "Missing $(KALKE_AUTH_DIR). Expected sibling repo kalke-auth."; exit 1; }
	@$(MAKE) -C "$(KALKE_AUTH_DIR)" up

.PHONY: auth-down
auth-down: ## Stop sibling kalke-auth
	@test -d "$(KALKE_AUTH_DIR)" || { echo "Missing $(KALKE_AUTH_DIR)"; exit 1; }
	@$(MAKE) -C "$(KALKE_AUTH_DIR)" down

.PHONY: auth-logs
auth-logs: ## Tail kalke-auth logs
	@test -d "$(KALKE_AUTH_DIR)" || { echo "Missing $(KALKE_AUTH_DIR)"; exit 1; }
	@$(MAKE) -C "$(KALKE_AUTH_DIR)" logs

.PHONY: auth-jwks
auth-jwks: ## Fetch JWKS via kalke-auth public proxy
	@test -d "$(KALKE_AUTH_DIR)" || { echo "Missing $(KALKE_AUTH_DIR)"; exit 1; }
	@$(MAKE) -C "$(KALKE_AUTH_DIR)" jwks

.PHONY: auth-token
auth-token: ## Print demo human access_token from kalke-auth (password grant)
	@test -d "$(KALKE_AUTH_DIR)" || { echo "Missing $(KALKE_AUTH_DIR)"; exit 1; }
	@$(MAKE) -C "$(KALKE_AUTH_DIR)" -s token

.PHONY: auth-m2m-token
auth-m2m-token: ## Print M2M access_token from kalke-auth (client_credentials)
	@test -d "$(KALKE_AUTH_DIR)" || { echo "Missing $(KALKE_AUTH_DIR)"; exit 1; }
	@$(MAKE) -C "$(KALKE_AUTH_DIR)" -s m2m-token

.PHONY: smoke-oidc
smoke-oidc: ## Human JWT smoke: password token → POST /v1/extract without file (expect 400)
	@token=$$($(MAKE) -C "$(KALKE_AUTH_DIR)" -s token); \
	curl -sS -o /dev/null -w "HTTP %{http_code}\n" -X POST \
		"http://localhost:$(APP_PORT)/v1/extract?doc_type=identity_document" \
		-H "Authorization: Bearer $$token"; \
	echo "(400 = JWT + scope OK; missing file)"

.PHONY: smoke-m2m
smoke-m2m: ## M2M JWT smoke: client_credentials → POST /v1/extract without file (expect 400)
	@token=$$($(MAKE) -C "$(KALKE_AUTH_DIR)" -s m2m-token); \
	curl -sS -o /dev/null -w "HTTP %{http_code}\n" -X POST \
		"http://localhost:$(APP_PORT)/v1/extract?doc_type=identity_document" \
		-H "Authorization: Bearer $$token"; \
	echo "(400 = JWT + scope OK; missing file)"

.PHONY: clean
clean: ## Remove local build artifacts
	rm -rf bin/ dist/ tmp/ .tmp/ coverage.out
