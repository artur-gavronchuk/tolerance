.PHONY: up down logs reset ps proof-image bot-image test check migrate run-api run-web connector scale

COMPOSE ?= $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo docker-compose)
-include .env
WEB_PORT ?= 3000
API_PORT ?= 8080
ARENA_ENV ?= development
ARENA_MONITORING ?= false
PROFILE := $(if $(filter production,$(ARENA_ENV)),--profile prod,) $(if $(filter true,$(ARENA_MONITORING)),--profile monitoring,)
# Production runs api/web/worker as their own services plus the prod
# override (host ports off api/web, replica counts from .env); see
# deploy/compose.prod.yml.
COMPOSE_FILES := -f docker-compose.yml $(if $(filter production,$(ARENA_ENV)),-f deploy/compose.prod.yml,)
# Group that owns docker.sock as containers see it: the worker container runs
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
	@if [ "$(ARENA_ENV)" = "production" ] && [ "$(ARENA_MONITORING)" = "true" ] && [ -z "$(GRAFANA_ADMIN_PASSWORD)" ]; then echo "refusing to start production with monitoring on and GRAFANA_ADMIN_PASSWORD empty in .env"; exit 1; fi
	@if [ "$(ARENA_ENV)" = "production" ] && [ "$(ARENA_DEV_LOGIN)" = "true" ]; then echo "refusing to start production with ARENA_DEV_LOGIN=true"; exit 1; fi
	DOCKER_GID=$(DOCKER_GID) $(COMPOSE) $(COMPOSE_FILES) $(PROFILE) up --build -d
	@echo
	@echo "  site:  http://localhost:$(WEB_PORT)"
	@echo "  api:   http://localhost:$(API_PORT)/healthz"
	@if [ "$(ARENA_MONITORING)" = "true" ]; then echo "  grafana: http://localhost:$(WEB_PORT)/grafana (or https://$(ARENA_DOMAIN)/grafana in prod)"; fi
	@echo "  logs:  make logs"

# The sandbox image for the first proof task; the worker container reaches
# the host daemon through docker.sock, so the image has to exist on the host.
proof-image:
	docker build -q -t arena-proof-go:1 backend/fixtures/proofs/go-fix-retry

# The runtime image tanks bots run in via match.DockerLauncher; the api container reaches the host daemon
# through docker.sock, so this image has to exist on the host too, same reasoning as proof-image above.
bot-image:
	docker build -q -t arena-bot-runtime:1 backend/internal/games/match/runtime

down:
	$(COMPOSE) $(COMPOSE_FILES) $(PROFILE) down

reset:
	@read -p "This deletes the database. Type yes: " a && [ "$$a" = "yes" ] && $(COMPOSE) $(COMPOSE_FILES) down -v

logs:
	$(COMPOSE) $(COMPOSE_FILES) logs -f --tail=100

ps:
	$(COMPOSE) $(COMPOSE_FILES) ps

# Change replica counts / worker concurrency without a rebuild, e.g.
# `make scale ARGS="workers=3 concurrency=2"`. Thin wrapper around
# deploy/scale.sh for a host reachable over ssh; see that script for what
# it actually runs.
scale:
	@test -n "$(HOST)" || (echo "usage: make scale HOST=<ssh-host> ARGS=\"workers=N web=N api=N concurrency=N\""; exit 1)
	./deploy/scale.sh $(HOST) $(ARGS)

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
