FROM node:26-alpine AS frontend-builder
WORKDIR /app
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.3-alpine3.24 AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy frontend build output to where embed.go expects it
COPY --from=frontend-builder /app/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o ai-gateway ./cmd/ai-gateway

FROM alpine:3.24
WORKDIR /opt/routex
# postgresql-client WAJIB ada: fitur Backup/Restore admin mengeksekusi
# pg_dump/psql dari PATH — tanpa paket ini fitur itu selalu 500 di Docker
# (audit3 V-DP-11). Paket client Alpine (versi server bawaannya lebih baru)
# tetap kompatibel dengan server PostgreSQL 16 di Compose: pg_dump/psql
# mendukung server yang lebih lama dari versinya sendiri.
RUN apk --no-cache add ca-certificates tzdata postgresql-client \
    && addgroup -g 10001 -S routex \
    && adduser -u 10001 -S -G routex -s /sbin/nologin routex
COPY --from=backend-builder --chmod=0555 /app/ai-gateway /opt/routex/ai-gateway
USER 10001:10001
ENV PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:${PORT}/healthz || exit 1
CMD ["/opt/routex/ai-gateway"]

