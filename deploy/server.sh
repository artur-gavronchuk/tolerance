#!/usr/bin/env bash
# Runs ON the server (/opt/tolerance) as `deploy`, called by
# .github/workflows/deploy.yml after it has synced compose.yml, Caddyfile and
# skills/ next to this file.
#
#   server.sh deploy <sha>   pull images at <sha>, dump the DB, migrate, swap
#                            api/web, health-check; on a failed health check
#                            the previous tag is started again (migrations
#                            are not reverted).
#   server.sh status         what runs and at which tag
set -euo pipefail
cd "$(dirname "$0")"

REGISTRY=ghcr.io/artur-gavronchuk
dc() { docker compose -f compose.yml "$@"; }

# setenv KEY VALUE: replace or append a line in .env.
setenv() {
  if grep -q "^$1=" .env; then sed -i "s|^$1=.*|$1=$2|" .env; else echo "$1=$2" >> .env; fi
}

# The api starts sandbox and bot containers from these fixed local names.
runtime_images() {
  docker tag "$REGISTRY/tolerance-bot-runtime:$1" arena-bot-runtime:1
  docker tag "$REGISTRY/tolerance-skill-go:$1" arena-skill-go:1
  docker tag "$REGISTRY/tolerance-skill-python:$1" arena-skill-python:1
  docker tag "$REGISTRY/tolerance-site:$1" arena-site:1
}

healthy() {
  for _ in $(seq 1 60); do
    if curl -fsS -o /dev/null http://127.0.0.1:8080/healthz && curl -fsS -o /dev/null http://127.0.0.1:3000/; then
      return 0
    fi
    sleep 2
  done
  return 1
}

cmd_deploy() {
  local tag=$1 prev
  prev=$(grep '^TAG=' .env | cut -d= -f2 || true)
  # Fair play hashes IPs with this key; it must survive restarts.
  grep -q '^ARENA_FAIRPLAY_SECRET=.' .env || setenv ARENA_FAIRPLAY_SECRET "$(openssl rand -hex 32)"

  echo "--- pull $tag"
  for img in backend web bot-runtime skill-go skill-python site; do
    docker pull -q "$REGISTRY/tolerance-$img:$tag"
  done
  runtime_images "$tag"

  # compose.yml needs TAG set for any command, postgres-only ones included.
  setenv TAG "$tag"

  echo "--- backup"
  mkdir -p backups
  dc up -d --wait postgres
  dc exec -T postgres pg_dump -U arena_migrate -d arena -Fc > "backups/pre-$tag.dump"
  ls -1t backups/pre-*.dump | tail -n +11 | xargs -r rm --

  echo "--- migrate"
  if ! dc run --rm migrate; then
    [ -n "$prev" ] && setenv TAG "$prev"
    echo "migrate failed; api and web untouched" >&2
    exit 1
  fi

  echo "--- up"
  # --remove-orphans drops services this file no longer has (the old
  # separate worker, canaries, the monitoring stack).
  dc up -d --remove-orphans
  docker exec "$(dc ps -q caddy)" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile

  if healthy; then
    [ -n "$prev" ] && [ "$prev" != "$tag" ] && echo "$prev" > .prev-tag
    echo "--- deployed $tag"
    docker image prune -f >/dev/null
    return 0
  fi

  echo "health check failed on $tag; logs:" >&2
  dc logs --tail=80 api web >&2 || true
  if [ -n "$prev" ] && [ "$prev" != "$tag" ]; then
    echo "--- rolling back to $prev" >&2
    setenv TAG "$prev"
    runtime_images "$prev" || true
    dc up -d
  fi
  exit 1
}

cmd_status() {
  echo "TAG=$(grep '^TAG=' .env | cut -d= -f2)"
  dc ps --format 'table {{.Service}}\t{{.Image}}\t{{.Status}}'
}

case "${1:-}" in
  deploy) cmd_deploy "${2:?usage: server.sh deploy <sha>}" ;;
  status) cmd_status ;;
  *) echo "usage: server.sh deploy <sha> | status" >&2; exit 2 ;;
esac
