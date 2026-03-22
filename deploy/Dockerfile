# ── Stage 1: Build ────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata wget

WORKDIR /app

# Зависимости в отдельном слое — кешируются при неизменном go.mod
COPY go.mod go.sum ./
RUN go mod download

# Исходники
COPY . .

# Билд: strip debug info (-w -s), версия из git tag
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o /bin/server ./cmd/server

# Скачиваем goose для миграций (вызывается в CI как ./migrate up)
RUN wget -qO /bin/migrate \
    https://github.com/pressly/goose/releases/download/v3.22.0/goose_linux_amd64 \
    && chmod +x /bin/migrate

# ── Stage 2: Runtime ──────────────────────────────────────────
# distroless/static: нет shell, минимум уязвимостей, но есть wget для healthcheck
FROM gcr.io/distroless/static-debian12

# Нужно для TLS (HTTPS клиенты) и временных зон
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Бинарник, мигратор и миграции
COPY --from=builder /bin/server /server
COPY --from=builder /bin/migrate /migrate
COPY --from=builder /app/migrations /migrations

EXPOSE 8080

ENTRYPOINT ["/server"]