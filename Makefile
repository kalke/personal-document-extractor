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

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: setup
setup: ## Create .env from .env.example if missing
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env — set GROQ_API_KEY before starting."; \
	else \
		echo ".env already exists"; \
	fi

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

.PHONY: up-fg
up-fg: check-env ## Build and start stack in foreground
	$(COMPOSE) up --build

.PHONY: down
down: ## Stop stack (keeps volumes)
	$(COMPOSE) down

.PHONY: destroy
destroy: ## Stop stack and delete volumes (destructive)
	$(COMPOSE) down -v

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
	go build -o /tmp/personal-document-extractor-apikey ./cmd/apikey

.PHONY: run
run: check-env ## Run API on the host (needs local Go, Postgres, Redis, poppler)
	go run ./cmd/api

.PHONY: migrate-local
migrate-local: ## Apply migrations with local Go (uses DATABASE_URL from .env)
	go run ./cmd/migrate

.PHONY: apikey
apikey: ## Create an API key: make apikey NAME=local SCOPES=extract:write
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	set +a; \
	go run ./cmd/apikey -name "$(or $(NAME),local)" -scopes "$(or $(SCOPES),extract:write)"

.PHONY: health
health: ## Curl /health
	@curl -sS "http://localhost:$(APP_PORT)/health"; echo

.PHONY: ready
ready: ## Curl /ready
	@curl -sS "http://localhost:$(APP_PORT)/ready"; echo

.PHONY: clean
clean: ## Remove local build artifacts
	rm -rf bin/ dist/ tmp/ .tmp/ coverage.out
