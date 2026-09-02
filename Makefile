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

# Test integrasi berbagi SATU database Postgres. Setiap paket memakai schema
# sementaranya sendiri dan memeriksa tidak ada yang tertinggal setelah selesai —
# pemeriksaan itu saling menuduh bila paket berjalan paralel. Karena itu -p 1.
TESTFLAGS := -count=1 -p 1

test: ## Semua test (unit + integrasi, butuh Postgres & Redis)
> $(GO) test $(TESTFLAGS) $(PKG)

test-unit: ## Hanya unit test (tanpa Postgres/Redis, bisa paralel)
> $(GO) test -count=1 -short $(PKG)

test-integration: ## Hanya test integrasi
> $(GO) test $(TESTFLAGS) -run Integration $(PKG)

race: ## Test dengan race detector
> $(GO) test $(TESTFLAGS) -race $(PKG)

cover: ## Test + laporan coverage
> $(GO) test $(TESTFLAGS) -coverprofile=coverage.out -covermode=atomic $(PKG)
> $(GO) tool cover -func=coverage.out | tail -1

# PERINGATAN: ini menghapus SEMUA schema berawalan test_, termasuk milik test yang
# sedang berjalan. Jalankan hanya saat tidak ada test yang aktif.
schemas-clean: ## Hapus schema test tertinggal (jangan jalankan saat test aktif)
> @set -a; . ./.env; set +a; \
>  psql "$$DATABASE_URL" -qtAX -c "select nspname from pg_namespace where nspname like 'test\\_%'" \
>  | while read -r s; do [ -n "$$s" ] && echo "menghapus $$s" && \
>    psql "$$DATABASE_URL" -q -c "drop schema \"$$s\" cascade"; done; true

clean: ## Hapus artefak build
> rm -f $(BINARY) coverage.out
> rm -rf web/dist/assets

db-shell: ## psql ke database aplikasi
> @set -a; . ./.env; set +a; psql "$$DATABASE_URL"

redis-shell: ## redis-cli ke instance aplikasi
> @set -a; . ./.env; set +a; redis-cli -u "$$REDIS_URL"
