API_DIR := apps/api
MIGRATIONS := db/migrations/0001_init.sql db/migrations/0002_milestone_b.sql
COMPOSE := infra/docker-compose.yml

WEB_DIR := apps/web

.PHONY: dev build test tidy migrate docker-up docker-down web-dev

dev:
	cd $(API_DIR) && go run ./cmd/api/...

build:
	cd $(API_DIR) && go build -ldflags="-w -s" -o bin/api ./cmd/api/...

test:
	cd $(API_DIR) && go test ./...

tidy:
	cd $(API_DIR) && go mod tidy

# Run as postgres superuser to create schema, RLS policies, and app role.
# Requires psql in PATH and the postgres container to be running.
migrate:
	for f in $(MIGRATIONS); do \
		PGPASSWORD=postgres psql -h localhost -U postgres -d saas_db -f $$f; \
	done

docker-up:
	docker compose -f $(COMPOSE) up -d

docker-down:
	docker compose -f $(COMPOSE) down

web-dev:
	cd $(WEB_DIR) && pnpm dev
