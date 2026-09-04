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

.PHONY: help deps fe build run dev fmt vet test test-unit test-integration race cover clean db-shell redis-shell require-db

help: ## Tampilkan daftar target
> @grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
>   | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

deps: ## Unduh dependensi Go dan npm
> $(GO) mod download
> @if [ -f web/package.json ]; then cd web && npm ci; fi

fe: ## Build frontend ke web/dist (di-embed ke binary)
> @if [ -f web/package.json ]; then cd web && npm run build && touch dist/.gitkeep; \
>  else echo "web/package.json belum ada — dilewati (Fase 12)"; fi

build: fe ## Build biner produksi ./ai-gateway dan perkakas rotasi
> CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/ai-gateway
> CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o routex-rotate ./cmd/routex-rotate
> @ls -lh $(BINARY) routex-rotate

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

# ENVLOAD memuat .env sebelum test dijalankan.
#
# Ini bukan kenyamanan. Test integrasi MELEWATI DIRINYA SENDIRI ketika DATABASE_URL tidak
# ada di environment, dan pelewatan itu tidak membuat gate merah — jadi tanpa baris ini
# `make test` melaporkan hijau sambil tidak menjalankan satu pun test yang menyentuh
# Postgres. Itu kegagalan terburuk yang bisa dimiliki sebuah gate: bukan salah menilai,
# melainkan tidak menilai apa pun sambil terlihat menilai.
#
# Terukur: `make race` tanpa ini selesai dalam 1 detik untuk paket repository dan
# melewatkan seluruh test integrasinya; dengan ini paket yang sama memakan 29 detik.
ENVLOAD = set -a; [ -f ./.env ] && . ./.env; set +a;

# require-db menggagalkan target integrasi lebih awal bila kredensialnya tidak ada, dengan
# pesan yang menyebut jalan keluarnya. Lebih baik merah dan jelas daripada hijau dan hampa.
.PHONY: require-db
require-db:
> @$(ENVLOAD) if [ -z "$$DATABASE_URL" ]; then \
>   echo "DATABASE_URL tidak diset dan .env tidak memuatnya."; \
>   echo "Test integrasi akan MELEWATI dirinya sendiri dan gate tetap hijau, jadi target ini berhenti di sini."; \
>   echo "Isi DATABASE_URL di ./.env, atau jalankan 'make test-unit' bila memang hanya unit test yang dimaksud."; \
>   exit 1; \
>  fi

test: require-db ## Semua test (unit + integrasi, butuh Postgres & Redis)
> $(ENVLOAD) $(GO) test $(TESTFLAGS) $(PKG)

test-unit: ## Hanya unit test (tanpa Postgres/Redis, bisa paralel)
> $(GO) test -count=1 -short $(PKG)

test-integration: require-db ## Hanya test integrasi
> $(ENVLOAD) $(GO) test $(TESTFLAGS) -run Integration $(PKG)

race: require-db ## Test dengan race detector
> $(ENVLOAD) $(GO) test $(TESTFLAGS) -race $(PKG)

cover: require-db ## Test + laporan coverage
> $(ENVLOAD) $(GO) test $(TESTFLAGS) -coverprofile=coverage.out -covermode=atomic $(PKG)
> $(GO) tool cover -func=coverage.out | tail -1

# PERINGATAN: ini menghapus SEMUA schema berawalan test_, termasuk milik test yang
# sedang berjalan. Jalankan hanya saat tidak ada test yang aktif.
schemas-clean: ## Hapus schema test tertinggal (jangan jalankan saat test aktif)
> @set -a; . ./.env; set +a; \
>  psql "$$DATABASE_URL" -qtAX -c "select nspname from pg_namespace where nspname like 'test\\_%'" \
>  | while read -r s; do [ -n "$$s" ] && echo "menghapus $$s" && \
>    psql "$$DATABASE_URL" -q -c "drop schema \"$$s\" cascade"; done; true

clean: ## Hapus artefak build
> rm -f $(BINARY) routex-rotate coverage.out
> rm -rf web/dist/assets
> touch web/dist/.gitkeep

db-shell: ## psql ke database aplikasi
> @set -a; . ./.env; set +a; psql "$$DATABASE_URL"

redis-shell: ## redis-cli ke instance aplikasi
> @set -a; . ./.env; set +a; redis-cli -u "$$REDIS_URL"
