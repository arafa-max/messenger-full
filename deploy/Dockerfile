# ── Stage 1: Build ────────────────────────────────────────────
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata wget

WORKDIR /app

COPY go.mod go.sum ./
RUN go env -w GOPROXY=direct GONOSUMDB=*
RUN go mod download

COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g ./cmd/server/main.go -o ./docs --parseDependency --parseInternal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o /bin/server ./cmd/server

RUN go install github.com/pressly/goose/v3/cmd/goose@latest
RUN cp $(go env GOPATH)/bin/goose /bin/migrate

# ── Stage 2: Runtime ──────────────────────────────────────────
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/server /server
COPY --from=builder /bin/migrate /migrate
COPY --from=builder /app/migrations /migrations
COPY entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]