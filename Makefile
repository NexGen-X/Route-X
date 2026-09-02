# Route-X — satu binary: API + dashboard + aset statis.
# Catatan: recipe prefix diganti '>' agar tidak bergantung pada karakter TAB.
.RECIPEPREFIX = >
.DEFAULT_GOAL := help

GO         ?= go
BINARY     := ai-gateway
PKG        := ./...
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILT_AT   := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
  -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.builtAt=$(BUILT_AT)

.PHONY: help deps fe build run dev fmt vet test test-unit test-integration race cover clean db-shell redis-shell

help: ## Tampilkan daftar target
> @grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
>   | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

deps: ## Unduh dependensi Go dan npm
> $(GO) mod download
> @if [ -f web/package.json ]; then cd web && npm ci; fi

fe: ## Build frontend ke web/dist (di-embed ke binary)
> @if [ -f web/package.json ]; then cd web && npm run build; \
>  else echo "web/package.json belum ada — dilewati (Fase 12)"; fi

build: fe ## Build biner produksi ./ai-gateway
> CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/ai-gateway
> @ls -lh $(BINARY)

run: build ## Build lalu jalankan
> ./$(BINARY)

dev: ## Jalankan dari source tanpa build frontend
> $(GO) run ./cmd/ai-gateway

fmt: ## Format kode
> $(GO) fmt $(PKG)

vet: ## Analisis statis bawaan Go
> $(GO) vet $(PKG)

test: ## Semua test (unit + integrasi)
> $(GO) test -count=1 $(PKG)

test-unit: ## Hanya unit test (tanpa Postgres/Redis)
> $(GO) test -count=1 -short $(PKG)

test-integration: ## Hanya test integrasi
> $(GO) test -count=1 -run Integration $(PKG)

race: ## Test dengan race detector
> $(GO) test -count=1 -race $(PKG)

cover: ## Test + laporan coverage
> $(GO) test -count=1 -coverprofile=coverage.out -covermode=atomic $(PKG)
> $(GO) tool cover -func=coverage.out | tail -1

clean: ## Hapus artefak build
> rm -f $(BINARY) coverage.out
> rm -rf web/dist/assets

db-shell: ## psql ke database aplikasi
> @set -a; . ./.env; set +a; psql "$$DATABASE_URL"

redis-shell: ## redis-cli ke instance aplikasi
> @set -a; . ./.env; set +a; redis-cli -u "$$REDIS_URL"
