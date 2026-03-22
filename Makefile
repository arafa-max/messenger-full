# Makefile — messenger-full
# Использование: make <команда>

.PHONY: help dev up down logs build test lint migrate backup shell clean

# Цвета для вывода
GREEN  := \033[0;32m
YELLOW := \033[1;33m
RESET  := \033[0m

help: ## Показать все команды
	@echo ""
	@echo "$(GREEN)messenger-full commands:$(RESET)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-18s$(RESET) %s\n", $$1, $$2}'
	@echo ""

# ── Разработка ───────────────────────────────────────────────
dev: ## Запустить локально (только инфра: postgres, redis, minio)
	docker compose up postgres redis minio -d
	@echo "$(GREEN)▶ Infrastructure ready. Run: go run ./cmd/server$(RESET)"

run: ## Запустить Go сервер локально
	go run ./cmd/server

# ── Docker ───────────────────────────────────────────────────
up: ## Поднять весь стек
	docker compose up -d

down: ## Остановить весь стек
	docker compose down

restart: ## Перезапустить app контейнер
	docker compose restart app

logs: ## Логи всех сервисов (follow)
	docker compose logs -f

logs-app: ## Логи только приложения
	docker compose logs -f app

build: ## Собрать Docker образ
	docker compose build --no-cache app

ps: ## Статус контейнеров
	docker compose ps

# ── Миграции ─────────────────────────────────────────────────
migrate-up: ## Применить все миграции
	goose -dir migrations postgres "$(shell grep DB_DSN .env | cut -d= -f2)" up

migrate-down: ## Откатить последнюю миграцию
	goose -dir migrations postgres "$(shell grep DB_DSN .env | cut -d= -f2)" down

migrate-status: ## Статус миграций
	goose -dir migrations postgres "$(shell grep DB_DSN .env | cut -d= -f2)" status

# ── Тесты ────────────────────────────────────────────────────
test: ## Запустить тесты
	go test ./... -v -race -timeout 60s

test-cover: ## Тесты с покрытием
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)▶ Open coverage.html$(RESET)"

lint: ## Запустить golangci-lint
	golangci-lint run ./...

# ── Кодогенерация ─────────────────────────────────────────────
sqlc: ## Перегенерировать sqlc queries
	sqlc generate

swagger: ## Перегенерировать swagger docs
	swag init -g cmd/server/main.go

# ── Бэкап ────────────────────────────────────────────────────
backup: ## Создать бэкап базы данных прямо сейчас
	docker compose exec postgres_backup sh /backup.sh

backup-list: ## Список бэкапов
	docker compose exec postgres_backup ls -lh /backups/

backup-restore: ## Восстановить бэкап (FILE=filename.sql.gz)
	@if [ -z "$(FILE)" ]; then echo "Укажи: make backup-restore FILE=messenger_2024-01-01.sql.gz"; exit 1; fi
	docker compose exec -T postgres_backup \
		sh -c "gunzip -c /backups/$(FILE) | psql -h postgres -U $$POSTGRES_USER -d $$POSTGRES_DB"

# ── TLS ───────────────────────────────────────────────────────
cert-init: ## Получить сертификат Let's Encrypt (первый раз)
	@if [ -z "$(DOMAIN)" ]; then echo "Укажи: make cert-init DOMAIN=example.com EMAIL=you@example.com"; exit 1; fi
	docker compose run --rm certbot certonly \
		--webroot -w /var/www/certbot \
		--email $(EMAIL) \
		--agree-tos --no-eff-email \
		-d $(DOMAIN) -d www.$(DOMAIN)

cert-renew: ## Обновить сертификаты
	docker compose run --rm certbot renew

# ── Отладка ───────────────────────────────────────────────────
shell-app: ## Shell внутри app контейнера
	docker compose exec app sh

shell-db: ## psql внутри postgres
	docker compose exec postgres psql -U $${DB_USER} -d $${DB_NAME}

shell-redis: ## redis-cli
	docker compose exec redis redis-cli

# ── Продакшн деплой ───────────────────────────────────────────
deploy: ## Задеплоить (pull + up --no-deps)
	docker compose pull app
	docker compose up -d --no-deps app

health: ## Проверить health endpoint
	curl -s http://localhost:8080/health | python3 -m json.tool

# ── Очистка ──────────────────────────────────────────────────
clean: ## Удалить volumes (ОСТОРОЖНО: теряются данные)
	@echo "$(YELLOW)⚠ Это удалит все данные! Продолжить? [y/N]$(RESET)" && read ans && [ $${ans:-N} = y ]
	docker compose down -v