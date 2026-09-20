.PHONY: all build build-web build-go test test-coverage migrate live-update status logs help

SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "v1.1.0")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

all: build

help:
	@echo "Route-X Management Commands:"
	@echo "  make build         - Build frontend UI and Go backend binary"
	@echo "  make build-go      - Build Go backend binary only"
	@echo "  make build-web     - Build React frontend only"
	@echo "  make test          - Run all Go tests"
	@echo "  make test-coverage - Run Go tests with coverage report"
	@echo "  make live-update   - Atomic live-rebuild and reload for staging/prod"
	@echo "  make status        - Show systemd status for all 5 services"
	@echo "  make logs          - Follow live Route-X gateway journalctl logs"

build-web:
	@echo "--> Building Web UI..."
	cd web && npm install --no-audit --no-fund && npm run build

build-go:
	@echo "--> Building Go binary ($(VERSION) @ $(COMMIT))..."
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/ai-gateway ./cmd/ai-gateway

build: build-web build-go

test:
	@echo "--> Running Go test suite..."
	go test -v ./...

test-coverage:
	@echo "--> Running test coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

live-update:
	@echo "--> Executing atomic live update..."
	./scripts/ops/update-live.sh

status:
	@echo "================ Native Services Status ================"
	@systemctl status routex postgresql redis-server caddy --lines=2 --no-pager || true

logs:
	@journalctl -fu routex
