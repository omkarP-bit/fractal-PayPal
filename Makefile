.PHONY: up down seed test demo lint migrate-up migrate-down test-negative test-concurrency

COMPOSE := docker-compose.yml
API_DIR := api
DB_URL := $(shell grep '^DATABASE_URL=' .env | cut -d= -f2-)

up:
	docker compose -f $(COMPOSE) up -d

down:
	docker compose -f $(COMPOSE) down

UP_MIGRATIONS := $(shell find $(API_DIR)/migrations -name '*.up.sql' | sort)
DOWN_MIGRATIONS := $(shell find $(API_DIR)/migrations -name '*.down.sql' | sort)

migrate-up:
	@set -e; \
	for f in $(UP_MIGRATIONS); do \
		echo "==> $$f"; \
		psql "$(DB_URL)" -f "$$f"; \
	done

migrate-down:
	@set -e; \
	for f in $(DOWN_MIGRATIONS); do \
		echo "==> $$f"; \
		psql "$(DB_URL)" -f "$$f" || true; \
	done

seed:
	@cd $(API_DIR) && DATABASE_URL="$(DB_URL)" go run -tags seed ./scripts/seed
	@psql "$(DB_URL)" -f $(API_DIR)/scripts/seed_demo_merchant.sql

test:
	@cd $(API_DIR) && go test ./...
	@cd investigator && python3 -m pytest -q

demo:
	@echo "=== FRACTAL Demo Script ==="
	@echo "1. Start stack:   make up"
	@echo "2. Migrations:    make migrate-up"
	@echo "3. Seed data:     make seed"
	@echo "4. Merchant store: http://localhost:8090"
	@echo "5. Web UI:        http://localhost:5173"

test-negative:
	@echo "=== Negative scenario checklist ==="
	@echo "1. Invalid signature webhook  -> HTTP 400"
	@echo "2. Amount mismatch            -> policy BLOCK"
	@echo "3. Refund without approval    -> REQUIRE_APPROVAL"
	@echo "4. Concurrent replays         -> one execution"
	@echo "5. Unknown policy version     -> BLOCK"
	@echo "6. Agent with fabricated evidence -> HTTP 422"
	@echo "7. Unauthorized case access   -> HTTP 404"

test-concurrency:
	@echo "=== Concurrency demo ==="
	@echo "Use /demo control panel or scripts/replay to fire parallel webhooks."
	@echo "Exactly one action_executions row per idempotency key."

lint:
	@cd $(API_DIR) && golangci-lint run ./... || true
	@cd investigator && ruff check .
	@cd web && npm run lint
