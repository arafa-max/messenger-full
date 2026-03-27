# ── Stage 1: Build ────────────────────────────────────────────
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata wget

WORKDIR /app

COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g ./cmd/server/main.go -o ./docs --parseDependency --parseInternal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o /bin/server ./cmd/server

RUN wget -qO /bin/migrate \
    https://github.com/pressly/goose/releases/download/v3.22.0/goose_linux_amd64 \
    && chmod +x /bin/migrate

# ── Stage 2: Runtime ──────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /bin/server /server
COPY --from=builder /bin/migrate /migrate
COPY --from=builder /app/migrations /migrations

EXPOSE 8080

ENTRYPOINT ["/server"]