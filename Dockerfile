FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy frontend build output to where embed.go expects it
COPY --from=frontend-builder /app/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ai-gateway ./cmd/ai-gateway

FROM alpine:latest
WORKDIR /opt/routex
RUN apk --no-cache add ca-certificates tzdata
COPY --from=backend-builder /app/ai-gateway .
EXPOSE 8080
CMD ["./ai-gateway"]
