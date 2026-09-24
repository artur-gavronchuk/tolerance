#!/usr/bin/env bash
# Superset workspace setup: https://docs.superset.sh/setup-teardown-scripts
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=lib/portalloc.sh
source .superset/lib/portalloc.sh

echo "==> Installing dependencies"

if command -v go >/dev/null 2>&1; then
  (cd backend && go mod download)
else
  echo "  go not found on PATH, skipping 'go mod download' (docker build will still fetch modules)" >&2
fi

if command -v pnpm >/dev/null 2>&1; then
  (cd frontend && pnpm install --frozen-lockfile)
elif command -v corepack >/dev/null 2>&1; then
  (cd frontend && corepack enable && pnpm install --frozen-lockfile)
else
  echo "  pnpm not found on PATH, skipping 'pnpm install' (docker build will still install deps)" >&2
fi

echo "==> Copying untracked env file"

if [ -n "${SUPERSET_ROOT_PATH:-}" ] && [ -f "$SUPERSET_ROOT_PATH/.env" ]; then
  cp "$SUPERSET_ROOT_PATH/.env" .env
elif [ ! -f .env ]; then
  cp .env.example .env
fi

# Idempotent KEY=value upsert in .env.
set_env_var() {
  local key="$1" value="$2"
  if grep -q "^${key}=" .env 2>/dev/null; then
    local tmp=".env.tmp.$$"
    awk -v k="$key" -v v="$value" -F= 'BEGIN{OFS="="} $1==k{$0=k"="v} {print}' .env >"$tmp" && mv "$tmp" .env
  else
    printf '%s=%s\n' "$key" "$value" >>.env
  fi
}

echo "==> Allocating dev-server ports for this workspace"

port_base=$(arena_allocate_port_base 0 1) || {
  echo "  Could not allocate a port base, leaving .env ports untouched" >&2
  port_base=""
}

if [ -n "$port_base" ]; then
  web_port=$((port_base + 0))
  api_port=$((port_base + 1))

  set_env_var WEB_PORT "$web_port"
  set_env_var API_PORT "$api_port"

  echo "  web=${web_port} api=${api_port}"
fi

echo "==> Setup complete"
