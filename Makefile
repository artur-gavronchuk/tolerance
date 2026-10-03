.PHONY: up down logs reset dev db docker migrate run-api run-web images test test-fast check connector

COMPOSE ?= $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo docker-compose)
-include .env
WEB_PORT ?= 3000
API_PORT ?= 8080
PG_PORT ?= 5432
# Group that owns docker.sock as containers see it: the api container runs as
# a non-root user and needs that group to reach the daemon for sandbox runs.
# Asked of the daemon itself because on macOS (colima, Docker Desktop) the
# socket lives in a VM. Only expanded by `up`; set DOCKER_GID in .env to override.
DOCKER_GID ?= $(shell docker run --rm -v /var/run/docker.sock:/var/run/docker.sock alpine:3.22 stat -c %g /var/run/docker.sock 2>/dev/null)

# Prebuilt connectors for GET /api/v1/connector/download in native runs; the
# api image builds its own.
CONNECTOR_DIR := $(CURDIR)/backend/.connector

# Everything in Docker.
up: .env docker images
	DOCKER_GID=$(DOCKER_GID) $(COMPOSE) up --build -d
	@echo
	@echo "  site:  http://localhost:$(WEB_PORT)"
	@echo "  api:   http://localhost:$(API_PORT)/healthz"
	@echo "  logs:  make logs"

down:
	$(COMPOSE) down

reset:
	$(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f --tail=100

# Fastest loop: postgres in Docker, api and web on the host (web hot-reloads;
# restart `make dev` after Go changes). Ctrl-C stops both.
dev: .env docker images db migrate connector
	@trap 'kill 0' INT TERM; \
	$(MAKE) --no-print-directory run-api & \
	$(MAKE) --no-print-directory run-web & \
	echo; echo "  site:  http://localhost:$(WEB_PORT)"; echo "  api:   http://localhost:$(API_PORT)/healthz"; echo; \
	wait

db: .env docker
	$(COMPOSE) up -d --wait postgres

migrate:
	cd backend && ARENA_MIGRATE_DATABASE_URL="postgres://arena_migrate:$(POSTGRES_PASSWORD)@127.0.0.1:$(PG_PORT)/arena?sslmode=disable" \
		ARENA_APP_ROLE_PASSWORD="$(ARENA_APP_ROLE_PASSWORD)" \
		ARENA_PROOFS_DIR=./fixtures/proofs ARENA_SKILLS_DIR=./fixtures/skills go run ./cmd/migrate

run-api:
	cd backend && ARENA_ADDR=127.0.0.1:$(API_PORT) ARENA_CONNECTOR_DIR=$(CONNECTOR_DIR) \
		ARENA_APP_DATABASE_URL="postgres://arena_app:$(ARENA_APP_ROLE_PASSWORD)@127.0.0.1:$(PG_PORT)/arena?sslmode=disable" \
		ARENA_DEV_LOGIN=$(or $(ARENA_DEV_LOGIN),true) ARENA_SANDBOX=$(or $(ARENA_SANDBOX),docker) ARENA_NO_LIMITS=$(or $(ARENA_NO_LIMITS),true) \
		ARENA_ADMIN_EMAILS="$(ARENA_ADMIN_EMAILS)" \
		ARENA_PUBLIC_URL=$(or $(ARENA_PUBLIC_URL),http://localhost:$(WEB_PORT)) \
		ARENA_GITHUB_CLIENT_ID="$(ARENA_GITHUB_CLIENT_ID)" ARENA_GITHUB_CLIENT_SECRET="$(ARENA_GITHUB_CLIENT_SECRET)" \
		ARENA_GOOGLE_CLIENT_ID="$(ARENA_GOOGLE_CLIENT_ID)" ARENA_GOOGLE_CLIENT_SECRET="$(ARENA_GOOGLE_CLIENT_SECRET)" \
		go run ./cmd/api

run-web:
	cd frontend && API_URL=http://127.0.0.1:$(API_PORT) pnpm dev -p $(WEB_PORT)

# Sandbox and bot runtime images the api starts containers from. Built only
# when missing; `docker rmi` one to force a rebuild.
images:
	@docker image inspect arena-proof-go:1 >/dev/null 2>&1 || docker build -q -t arena-proof-go:1 backend/fixtures/proofs/go-fix-retry
	@docker image inspect arena-skill-go:1 >/dev/null 2>&1 || docker build -q -t arena-skill-go:1 backend/fixtures/skills/go
	@docker image inspect arena-skill-python:1 >/dev/null 2>&1 || docker build -q -t arena-skill-python:1 backend/fixtures/skills/python
	@docker image inspect arena-bot-runtime:1 >/dev/null 2>&1 || docker build -q -t arena-bot-runtime:1 backend/internal/games/match/runtime

docker:
	@docker info >/dev/null 2>&1 || (command -v colima >/dev/null && colima start) || (echo "Docker is not running"; exit 1)

# The connector for this machine, served by GET /api/v1/connector/download in
# native runs (the api image builds all four platforms itself).
connector:
	cd backend && CGO_ENABLED=0 go build -trimpath -o $(CONNECTOR_DIR)/arena-$$(go env GOOS)-$$(go env GOARCH) ./cmd/arena

# Everyday check: vet, the few remaining tests (integration ones skip without Docker), typecheck.
test-fast:
	cd backend && go vet ./... && go test ./...
	cd frontend && pnpm typecheck

# Everything, the way CI runs it.
test:
	cd backend && ARENA_TEST_REQUIRE_DOCKER=1 go test -race ./...
	cd frontend && pnpm typecheck && pnpm build

check:
	cd backend && go vet ./... && test -z "$$(gofmt -l .)"

.env:
	cp .env.example .env
