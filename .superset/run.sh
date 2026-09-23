#!/usr/bin/env bash
# Superset workspace run: https://docs.superset.sh/setup-teardown-scripts
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

docker info >/dev/null 2>&1 || (command -v colima >/dev/null 2>&1 && colima start) || {
  echo "Docker is not running" >&2
  exit 1
}

COMPOSE="docker compose"
docker compose version >/dev/null 2>&1 || COMPOSE="docker-compose"

web_port=$(grep '^WEB_PORT=' .env 2>/dev/null | cut -d= -f2)
api_port=$(grep '^API_PORT=' .env 2>/dev/null | cut -d= -f2)
echo "  site:  http://localhost:${web_port:-3000}"
echo "  api:   http://localhost:${api_port:-8080}/api/v1/competitions"
echo "  login: admin@arena.local / password   (dev@arena.local, dev2@arena.local)"
echo

exec $COMPOSE up --build
