#!/usr/bin/env bash
# shellcheck disable=SC2087,SC2029
# SC2087/SC2029: every heredoc/quoted remote command below that expands
# $remote_dir (etc.) on the client side does so deliberately — it's a fixed
# local constant, not remote-influenced input, and heredocs that need
# fully remote evaluation are quoted (<<'REMOTE_ENV') instead.
#
# Ship the repo to a bootstrapped server and bring it up.
#
# Usage: deploy/deploy.sh <ssh-host>
#   ssh-host is anything `ssh` accepts (an alias from ~/.ssh/config is
#   easiest: user, key and port already resolved there).
#
# What it does:
#   1. tars the repo (excluding local/dev-only paths) and extracts it into
#      /opt/tolerance on the host;
#   2. on first run only, writes /opt/tolerance/.env with random passwords
#      and capacity settings sized from the host's nproc/RAM (never
#      overwrites an existing .env — see size_hint below);
#   3. runs `make up` under setsid/nohup on the host so an SSH drop can't
#      kill a long build (this has happened before), and follows the log;
#   4. waits for containers to report healthy, then curls the public URL;
#   5. if CLOUDFLARE_API_TOKEN/CLOUDFLARE_ZONE_ID are set in the LOCAL shell
#      (never written to the server), purges the Cloudflare cache for the
#      connector download URLs so new connector binaries go out.
set -euo pipefail

host="${1:?usage: deploy/deploy.sh <ssh-host>}"
remote_dir=/opt/tolerance
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ssh_opts=(-o ServerAliveInterval=15 -o ServerAliveCountMax=6)

log() { echo "[deploy] $*"; }

# --- 1. ship the repo ---------------------------------------------------------
log "packing repo (excluding .git, node_modules, .next, .env, .superset, backend/.connector)"
tarball="$(mktemp -t tolerance-deploy.XXXXXX).tar.gz"
trap 'rm -f "$tarball"' EXIT

# COPYFILE_DISABLE stops macOS bsdtar from writing AppleDouble (._*) sidecar
# files into the archive in the first place; --no-xattrs/--no-mac-metadata
# belt-and-braces on top for whichever tar is actually on PATH. A prior
# deploy shipped 268 ._* files this way and broke goose on ._00002_schema.sql.
tar_extra_args=()
if tar --help 2>&1 | grep -q -- '--no-xattrs'; then
	tar_extra_args+=(--no-xattrs)
fi
if tar --help 2>&1 | grep -q -- '--no-mac-metadata'; then
	tar_extra_args+=(--no-mac-metadata)
fi

COPYFILE_DISABLE=1 tar czf "$tarball" \
	"${tar_extra_args[@]}" \
	-C "$repo_root" \
	--exclude='.git' \
	--exclude='node_modules' \
	--exclude='.next' \
	--exclude='.env' \
	--exclude='.superset' \
	--exclude='backend/.connector' \
	--exclude='._*' \
	--exclude='.DS_Store' \
	.

log "uploading to $host:$remote_dir"
ssh "${ssh_opts[@]}" "$host" "mkdir -p $remote_dir"
scp "${ssh_opts[@]}" "$tarball" "$host:/tmp/tolerance-deploy.tar.gz"
ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE_EXTRACT
set -euo pipefail
tar xzf /tmp/tolerance-deploy.tar.gz -C "$remote_dir"
rm -f /tmp/tolerance-deploy.tar.gz
# Belt-and-braces: delete any AppleDouble files that made it through anyway.
find "$remote_dir" -name '._*' -type f -delete
find "$remote_dir" -name '.DS_Store' -type f -delete
REMOTE_EXTRACT

# --- 2. .env: create once, size from the host, never overwrite ---------------
log "checking $remote_dir/.env"
# shellcheck disable=SC2087
ssh "${ssh_opts[@]}" "$host" bash -s <<'REMOTE_ENV'
set -euo pipefail
cd /opt/tolerance

nproc_count=$(nproc)
ram_kb=$(awk '/MemTotal/{print $2}' /proc/meminfo)
ram_gb=$((ram_kb / 1024 / 1024))
[ "$ram_gb" -lt 1 ] && ram_gb=1

# Formulas (documented in README.md's "Сервер" section too):
#   workers            = max(1, nproc-2), capped by (RAM_GB-6) since each
#                         sandbox run can use up to ~1 GB and we reserve
#                         ~6 GB for postgres/api/web/OS.
#   ARENA_DB_POOL_MAX   = 4 per core, capped at 100.
#   PG_MAX_CONNECTIONS  = 3x the pool size, floor 200, so scaling api/worker
#                         to a couple of replicas doesn't need a bump too.
#   PG_SHARED_BUFFERS   = 25% of RAM.
#   PG_EFFECTIVE_CACHE_SIZE = 60% of RAM.
#   PG_WORK_MEM         = 4MB, doubling per doubling of RAM past 8GB.
#   PG_MAINTENANCE_WORK_MEM = 16x PG_WORK_MEM, capped at 1GB.
workers_by_cpu=$((nproc_count > 2 ? nproc_count - 2 : 1))
workers_by_ram=$((ram_gb > 6 ? ram_gb - 6 : 1))
worker_concurrency=$((workers_by_cpu < workers_by_ram ? workers_by_cpu : workers_by_ram))
[ "$worker_concurrency" -lt 1 ] && worker_concurrency=1

db_pool_max=$((nproc_count * 4))
[ "$db_pool_max" -gt 100 ] && db_pool_max=100
[ "$db_pool_max" -lt 10 ] && db_pool_max=10

pg_max_connections=$((db_pool_max * 3))
[ "$pg_max_connections" -lt 200 ] && pg_max_connections=200

shared_buffers_mb=$((ram_gb * 1024 / 4))
effective_cache_mb=$((ram_gb * 1024 * 60 / 100))
work_mem_mb=4
[ "$ram_gb" -gt 8 ] && work_mem_mb=8
[ "$ram_gb" -gt 32 ] && work_mem_mb=16
maintenance_work_mem_mb=$((work_mem_mb * 16))
[ "$maintenance_work_mem_mb" -gt 1024 ] && maintenance_work_mem_mb=1024

docker_gid=$(stat -c %g /var/run/docker.sock)

print_sizing() {
	echo "  nproc=$nproc_count ram_gb=$ram_gb -> ARENA_WORKER_CONCURRENCY=$worker_concurrency ARENA_DB_POOL_MAX=$db_pool_max" \
		"PG_MAX_CONNECTIONS=$pg_max_connections PG_SHARED_BUFFERS=${shared_buffers_mb}MB" \
		"PG_EFFECTIVE_CACHE_SIZE=${effective_cache_mb}MB PG_WORK_MEM=${work_mem_mb}MB" \
		"PG_MAINTENANCE_WORK_MEM=${maintenance_work_mem_mb}MB"
}

if [ -f .env ]; then
	echo "[deploy] .env already exists, leaving it as is. Host-sized values would be:"
	print_sizing
else
	echo "[deploy] writing new .env, sized from this host:"
	print_sizing
	cat >.env <<ENV
# Written by deploy/deploy.sh on first deploy. See .env.example for what
# every variable means; re-run deploy/scale.sh to change replica counts or
# worker concurrency live, or edit this file directly and \`make up\` again.
ARENA_ENV=production
ARENA_DOMAIN=tolerance.cc
ARENA_SECURE_COOKIES=true
POSTGRES_PASSWORD=$(openssl rand -hex 24)
ARENA_APP_ROLE_PASSWORD=$(openssl rand -hex 24)
ARENA_ADMIN_EMAILS=
ARENA_CONTACT_EMAIL=hello@tolerance.cc
WEB_PORT=3000
API_PORT=8080
DOCKER_GID=$docker_gid

ARENA_WORKER_CONCURRENCY=$worker_concurrency
ARENA_DB_POOL_MAX=$db_pool_max
PG_MAX_CONNECTIONS=$pg_max_connections
PG_SHARED_BUFFERS=${shared_buffers_mb}MB
PG_EFFECTIVE_CACHE_SIZE=${effective_cache_mb}MB
PG_WORK_MEM=${work_mem_mb}MB
PG_MAINTENANCE_WORK_MEM=${maintenance_work_mem_mb}MB
PG_SHM_SIZE=${shared_buffers_mb}mb

API_REPLICAS=1
WEB_REPLICAS=1
WORKER_REPLICAS=1

ARENA_MONITORING=true
GRAFANA_ADMIN_PASSWORD=$(openssl rand -hex 12)
PROM_RETENTION=30d

ARENA_RATE_IP_RPS=20
ARENA_RATE_IP_BURST=60
ARENA_RATE_KEY_RPS=5
ARENA_TURNSTILE_SITE_KEY=
ARENA_TURNSTILE_SECRET=

PG_BIND=127.0.0.1
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
ENV
	chmod 600 .env
fi
REMOTE_ENV

# --- 3. render Telegram alerting (no-op if the vars aren't set) --------------
# Unquoted heredoc: $remote_dir is substituted locally before sending, the
# rest of the script runs entirely on the remote end. This avoids ssh's
# habit of rejoining/re-splitting separate command-line arguments, which
# bit an earlier version of this script (`bash -c "'...'"` nesting).
log "rendering Grafana alerting provisioning from .env"
ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE_TELEGRAM
set -euo pipefail
cd $remote_dir
set -a
. ./.env
set +a
./deploy/render-telegram-alerting.sh ./deploy/monitoring/grafana/provisioning/alerting
REMOTE_TELEGRAM

# --- 4. bring the stack up, resilient to an SSH drop -------------------------
log "starting make up on $host (backgrounded, logs to $remote_dir/deploy.log)"
ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE_UP
set -euo pipefail
cd $remote_dir
rm -f deploy.pid
setsid nohup env ARENA_ENV=production make up \
	>deploy.log 2>&1 </dev/null &
disown
echo \$! >deploy.pid
REMOTE_UP

log "following $remote_dir/deploy.log (Ctrl-C only stops watching, not the remote build)"
ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE_FOLLOW
cd $remote_dir
pid=\$(cat deploy.pid)
tail -n +1 -f deploy.log &
tail_pid=\$!
while kill -0 "\$pid" 2>/dev/null; do sleep 2; done
kill "\$tail_pid" 2>/dev/null || true
wait "\$tail_pid" 2>/dev/null || true
REMOTE_FOLLOW

# --- 5. wait for containers to be healthy, then check the public URL --------
log "waiting for containers to report healthy"
for _ in $(seq 1 60); do
	unhealthy=$(ssh "${ssh_opts[@]}" "$host" "cd $remote_dir && docker compose ps --format '{{.Health}}' 2>/dev/null | grep -vE '^(healthy|)$' || true")
	if [ -z "$unhealthy" ]; then
		break
	fi
	sleep 5
done

domain=$(ssh "${ssh_opts[@]}" "$host" "grep -E '^ARENA_DOMAIN=' $remote_dir/.env | cut -d= -f2")
log "checking https://$domain/ and /api/v1/healthz"
site_code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "https://$domain/" || echo "ERR")
api_code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "https://$domain/api/v1/healthz" || echo "ERR")

grafana_pass=$(ssh "${ssh_opts[@]}" "$host" "grep -E '^GRAFANA_ADMIN_PASSWORD=' $remote_dir/.env | cut -d= -f2" || true)

echo
echo "== deploy summary =="
echo "  site:            https://$domain/  (HTTP $site_code)"
echo "  api healthz:      https://$domain/api/v1/healthz  (HTTP $api_code)"
echo "  grafana:          https://$domain/grafana  (user: admin, password: $grafana_pass)"
echo "  remote log:       $host:$remote_dir/deploy.log"
echo "  containers:"
ssh "${ssh_opts[@]}" "$host" "cd $remote_dir && docker compose ps" | sed 's/^/    /'

if [ "$site_code" != "200" ] || [ "$api_code" != "200" ]; then
	echo
	echo "WARNING: site or api did not return 200 — check deploy.log and docker compose ps above." >&2
fi

# --- 6. optional: purge Cloudflare cache for the connector download URL -----
if [ -n "${CLOUDFLARE_API_TOKEN:-}" ] && [ -n "${CLOUDFLARE_ZONE_ID:-}" ]; then
	log "purging Cloudflare cache for connector downloads"
	files=()
	for os in darwin linux; do
		for arch in amd64 arm64; do
			files+=("\"https://$domain/api/v1/connector/download?os=$os&arch=$arch\"")
		done
	done
	files_json=$(
		IFS=,
		echo "[${files[*]}]"
	)
	curl -sS -X POST "https://api.cloudflare.com/client/v4/zones/${CLOUDFLARE_ZONE_ID}/purge_cache" \
		-H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" \
		-H "Content-Type: application/json" \
		--data "{\"files\": $files_json}" | jq -c '{success, errors}'
else
	log "CLOUDFLARE_API_TOKEN/CLOUDFLARE_ZONE_ID not set locally, skipping cache purge"
fi

log "done"
