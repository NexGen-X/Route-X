#!/usr/bin/env bash
# ==============================================================================
# Route-X Personal AI Gateway — Skrip Penghenti Layanan Docker Compose
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$ROOT_DIR"

echo "Menghentikan seluruh layanan kontainer Route-X (Gateway, PostgreSQL, Redis)..."
docker compose down

echo "Seluruh layanan Route-X telah berhasil dihentikan dengan aman."
