.PHONY: run migrate test check up down

# Local development defaults; override any of these in the environment.
FORGE_ADDR ?= 127.0.0.1:8080
FORGE_APP_DATABASE_URL ?= postgres://forge_app:$${FORGE_APP_ROLE_PASSWORD}@127.0.0.1:5432/forge?sslmode=disable
FORGE_MIGRATE_DATABASE_URL ?= postgres://forge_migrate:forge_migrate_dev_password@127.0.0.1:5432/forge?sslmode=disable

up:
	docker compose up -d postgres

down:
	docker compose down

migrate:
	FORGE_MIGRATE_DATABASE_URL="$(FORGE_MIGRATE_DATABASE_URL)" go run ./cmd/migrate

run:
	FORGE_ADDR="$(FORGE_ADDR)" FORGE_APP_DATABASE_URL="$(FORGE_APP_DATABASE_URL)" go run ./cmd/api

test:
	go test -race ./...

check:
	go vet ./...
	gofmt -l .
