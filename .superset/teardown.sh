#!/usr/bin/env bash
# Superset workspace teardown: https://docs.superset.sh/setup-teardown-scripts
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=lib/portalloc.sh
source .superset/lib/portalloc.sh

echo "==> Stopping the dev server stack"

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  COMPOSE="docker compose"
  docker compose version >/dev/null 2>&1 || COMPOSE="docker-compose"
  $COMPOSE down -v --remove-orphans
else
  echo "  Docker not available, nothing to stop" >&2
fi

echo "==> Releasing allocated ports"
arena_release_port_base || echo "  Could not release port allocation for $PWD" >&2

echo "==> Teardown complete"
