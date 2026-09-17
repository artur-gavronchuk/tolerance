.PHONY: run migrate seed test check up down

ARENA_ADDR ?= 127.0.0.1:8080
ARENA_APP_DATABASE_URL ?= postgres://arena_app:$${ARENA_APP_ROLE_PASSWORD}@127.0.0.1:5432/arena?sslmode=disable
ARENA_MIGRATE_DATABASE_URL ?= postgres://arena_migrate:arena_migrate_dev_password@127.0.0.1:5432/arena?sslmode=disable

up:
	docker compose up -d postgres

down:
	docker compose down

migrate:
	ARENA_MIGRATE_DATABASE_URL="$(ARENA_MIGRATE_DATABASE_URL)" go run ./cmd/migrate

seed:
	ARENA_MIGRATE_DATABASE_URL="$(ARENA_MIGRATE_DATABASE_URL)" go run ./cmd/seed

run:
	ARENA_ADDR="$(ARENA_ADDR)" ARENA_APP_DATABASE_URL="$(ARENA_APP_DATABASE_URL)" go run ./cmd/api

test:
	go test -race ./...

check:
	go vet ./...
	gofmt -l .
