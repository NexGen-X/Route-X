# ==============================================================================
# Route-X AI Gateway — Multi-Stage Production Dockerfile
# ==============================================================================

# ------------------------------------------------------------------------------
# Stage 1: Build Frontend Single Page Application (React 18 + Vite + Tailwind)
# ------------------------------------------------------------------------------
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web

# Pasang dependensi terlebih dahulu untuk memanfaatkan caching Docker layer
COPY web/package.json web/package-lock.json ./
RUN npm ci --silent

# Salin sumber frontend dan lakukan kompilasi produksi ke web/dist
COPY web/ ./
RUN npm run build

# ------------------------------------------------------------------------------
# Stage 2: Build Biner Tunggal Go dengan Aset Frontend Tersemat
# ------------------------------------------------------------------------------
FROM golang:alpine AS backend-builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

# Cache dependensi Go
COPY go.mod go.sum ./
RUN go mod download

# Salin seluruh kode sumber backend
COPY . .

# Ambil bundel hasil kompilasi frontend dari Stage 1
COPY --from=frontend-builder /app/web/dist ./web/dist

# Kompilasi biner statis tanpa dependensi CGO
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=docker -X main.builtAt=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    -o /ai-gateway ./cmd/ai-gateway

# ------------------------------------------------------------------------------
# Stage 3: Runtime Image Minimalis & Aman (Non-Root User)
# ------------------------------------------------------------------------------
FROM alpine:3.20 AS runner

# Pasang sertifikat CA untuk panggilan HTTPS ke upstream provider dan data zona waktu
RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -g 10001 -S routex && \
    adduser -u 10001 -S routex -G routex -h /var/lib/route-x

# Salin biner hasil kompilasi dari Stage 2
COPY --from=backend-builder /ai-gateway /usr/local/bin/ai-gateway

# Direktori kerja aman non-root
WORKDIR /var/lib/route-x
USER routex:routex

EXPOSE 8080

# Pemeriksaan kesehatan berkala kontainer
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f -s http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/ai-gateway"]
