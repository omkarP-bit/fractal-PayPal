.PHONY: up down seed test demo lint migrate-up migrate-down

# Compose files
COMPOSE := docker-compose.yml

up:
	docker compose -f $(COMPOSE) up -d

down:
	docker compose -f $(COMPOSE) down

migrate-up:
	@cd fractal/api && go run github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database "$$(grep DATABASE_URL .env | cut -d'=' -f2)" up

migrate-down:
	@cd fractal/api && go run github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database "$$(grep DATABASE_URL .env | cut -d'=' -f2)" down

seed:
	@cd fractal/api && go run ./scripts/seed

test:
	@cd fractal/api && go test ./...
	@cd fractal/investigator && pytest

demo:
	@echo "=== FRACTAL Demo Script ==="
	@echo "1. Start stack: make up"
	@echo "2. Run migrations: make migrate-up"
	@echo "3. Seed data: make seed"
	@echo "4. Open merchant store: http://localhost:8090"
	@echo "5. Open web UI: http://localhost:5173"
	@echo "6. Set merchant handler to BROKEN"
	@echo "7. Pay through PayPal Sandbox"
	@echo "8. Open case as customer, reconcile as merchant"
	@echo "9. Run negative scenarios: make test-negative"

seed:
	@echo "=== Seeding demo data ==="
	@cd fractal/api && DATABASE_URL=$$(grep DATABASE_URL .env | cut -d'=' -f2) go run -tags seed ./scripts/seed
	@cd fractal/api && DATABASE_URL=$$(grep DATABASE_URL .env | cut -d'=' -f2) psql -f scripts/seed_demo_merchant.sql

test-negative:
	@echo "=== Running negative test scenarios ==="
	@echo "1. Invalid signature webhook -> 400 rejected"
	@echo "2. Amount mismatch -> BLOCK"
	@echo "3. Refund without approval -> REQUIRE_APPROVAL"
	@echo "4. Concurrent replays -> exactly one execution"
	@echo "5. Unknown policy version -> BLOCK"
	@echo "6. Agent with fabricated evidence -> REJECTED"
	@echo "Run manually against http://localhost:8080"

test-concurrency:
	@echo "=== Running concurrency test ==="
	@curl -s -X POST http://localhost:8080/api/v1/webhooks/paypal/00000000-0000-0000-0000-0000000000b1 \
	  -d '{"id":"evt_test","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"cap_test"}}' \
	  -H "Paypal-Transmission-Id: test" \
	  -H "Paypal-Transmission-Time: 2026-11-07T10:00:00Z" \
	  -H "Paypal-Transmission-Sig: test" \
	  -H "Paypal-Cert-Url: https://api-m.sandbox.paypal.com/cert" \
	  -H "Paypal-Auth-Algo: SHA256withRSA"

lint:
	@cd fractal/api && golangci-lint run ./... || true
	@cd fractal/investigator && ruff check .
	@cd fractal/web && npm run lint
