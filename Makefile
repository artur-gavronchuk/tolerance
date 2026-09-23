.PHONY: up down logs reset ps

# `docker compose` (plugin) when present, otherwise the standalone docker-compose binary.
COMPOSE ?= $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo docker-compose)

-include .env
WEB_PORT ?= 3000
API_PORT ?= 8080

# Site + API + PostgreSQL + Dex (local OIDC). Starts Colima on a Mac if Docker is down.
up: .env
	@docker info >/dev/null 2>&1 || (command -v colima >/dev/null && colima start) || (echo "Docker is not running"; exit 1)
	$(COMPOSE) up --build -d
	@echo
	@echo "  site:  http://localhost:$(WEB_PORT)"
	@echo "  api:   http://localhost:$(API_PORT)/api/v1/competitions"
	@echo "  login: admin@arena.local / password   (dev@arena.local, dev2@arena.local)"
	@echo "  logs:  make logs"

down:
	$(COMPOSE) down

# Wipe the database too.
reset:
	$(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

.env:
	cp .env.example .env
