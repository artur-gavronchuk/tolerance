.PHONY: up down logs reset ps proof-image bot-image test check migrate run-api run-web connector

COMPOSE ?= $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo docker-compose)
-include .env
WEB_PORT ?= 3000
API_PORT ?= 8080
ARENA_ENV ?= development
PROFILE := $(if $(filter production,$(ARENA_ENV)),--profile prod,)
# Group that owns docker.sock as containers see it: the api container runs
# as a non-root user and needs that group to reach the daemon. Asked of the
# daemon itself because on macOS (colima, Docker Desktop) the socket lives in
# a VM and the host's groups say nothing about it. Only expanded by `up`;
# set DOCKER_GID in .env to override. compose falls back to 999 when empty.
DOCKER_GID ?= $(shell docker run --rm -v /var/run/docker.sock:/var/run/docker.sock alpine:3.22 stat -c %g /var/run/docker.sock 2>/dev/null)

# Prebuilt connectors for GET /api/v1/connector/download in native runs; the
# api image builds its own.
CONNECTOR_DIR := $(CURDIR)/backend/.connector

up: .env proof-image bot-image
	@docker info >/dev/null 2>&1 || (command -v colima >/dev/null && colima start) || (echo "Docker is not running"; exit 1)
	@if [ "$(ARENA_ENV)" = "production" ] && grep -q "dev_password" .env; then echo "refusing to start production with dev passwords in .env"; exit 1; fi
	@if [ "$(ARENA_ENV)" = "production" ] && [ "$(ARENA_DEV_LOGIN)" = "true" ]; then echo "refusing to start production with ARENA_DEV_LOGIN=true"; exit 1; fi
	DOCKER_GID=$(DOCKER_GID) $(COMPOSE) $(PROFILE) up --build -d
	@echo
	@echo "  site:  http://localhost:$(WEB_PORT)"
	@echo "  api:   http://localhost:$(API_PORT)/healthz"
	@echo "  logs:  make logs"

# The sandbox image for the first proof task; the api container reaches the
# host daemon through docker.sock, so the image has to exist on the host.
proof-image:
	docker build -q -t arena-proof-go:1 backend/fixtures/proofs/go-fix-retry

# The runtime image tanks bots run in via match.DockerLauncher; the api container reaches the host daemon
# through docker.sock, so this image has to exist on the host too, same reasoning as proof-image above.
bot-image:
	docker build -q -t arena-bot-runtime:1 backend/internal/games/match/runtime

down:
	$(COMPOSE) $(PROFILE) down

reset:
	@read -p "This deletes the database. Type yes: " a && [ "$$a" = "yes" ] && $(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

# Native development (postgres from compose, api and web on the host).
migrate:
	cd backend && ARENA_MIGRATE_DATABASE_URL="postgres://arena_migrate:$(POSTGRES_PASSWORD)@127.0.0.1:5432/arena?sslmode=disable" \
		ARENA_APP_ROLE_PASSWORD="$(ARENA_APP_ROLE_PASSWORD)" ARENA_PROOFS_DIR=./fixtures/proofs go run ./cmd/migrate

run-api:
	cd backend && ARENA_ADDR=127.0.0.1:$(API_PORT) ARENA_CONNECTOR_DIR=$(CONNECTOR_DIR) \
		ARENA_APP_DATABASE_URL="postgres://arena_app:$(ARENA_APP_ROLE_PASSWORD)@127.0.0.1:5432/arena?sslmode=disable" \
		ARENA_DEV_LOGIN=$(or $(ARENA_DEV_LOGIN),true) go run ./cmd/api

run-web:
	cd frontend && API_URL=http://127.0.0.1:$(API_PORT) pnpm dev -p $(WEB_PORT)

connector:
	cd backend && for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do \
		CGO_ENABLED=0 GOOS=$${target%/*} GOARCH=$${target#*/} go build -trimpath \
			-o $(CONNECTOR_DIR)/arena-$${target%/*}-$${target#*/} ./cmd/arena || exit 1; \
	done

test:
	cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
	cd frontend && pnpm typecheck && pnpm build

check:
	cd backend && go vet ./... && test -z "$$(gofmt -l .)"

.env:
	cp .env.example .env
