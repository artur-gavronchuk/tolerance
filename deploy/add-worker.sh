#!/usr/bin/env bash
# shellcheck disable=SC2087,SC2029
# Add a remote sandbox-worker-only host: bootstraps it, ships the repo,
# builds the backend + proof images there, writes a worker-only .env
# pointing at the main host's Postgres over a private network, brings up
# deploy/compose.worker.yml, and registers its metrics with the main host's
# Prometheus via file_sd.
#
# Usage: deploy/add-worker.sh <new-ssh-host> <main-ssh-host> [concurrency]
#
# Requirements (see README.md "Сервер" -> "Как масштабировать"):
#   - new-ssh-host and main-ssh-host must be able to reach each other over a
#     PRIVATE network (e.g. a Hetzner Cloud Network) — this script does not
#     set one up.
#   - the main host's .env must have PG_BIND set to its address on that
#     private network (not 127.0.0.1) and PG_ALLOW_CIDR set to the private
#     subnet, and deploy/cf-origin-lock.sh must have been run there so
#     5432/3100 are reachable from the new host but not the public internet.
set -euo pipefail

new_host="${1:?usage: deploy/add-worker.sh <new-ssh-host> <main-ssh-host> [concurrency]}"
main_host="${2:?usage: deploy/add-worker.sh <new-ssh-host> <main-ssh-host> [concurrency]}"
concurrency="${3:-1}"

remote_dir=/opt/tolerance
main_remote_dir=/opt/tolerance
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ssh_opts=(-o IPQoS=none -o ServerAliveInterval=15 -o ServerAliveCountMax=6)
# A short, filesystem/Prometheus-label-safe name for this worker, used for
# the target files registered with the main host's Prometheus.
worker_name="$(echo "$new_host" | tr -c 'A-Za-z0-9' '-' | sed 's/^-*//;s/-*$//')"

log() { echo "[add-worker] $*"; }

# --- 1. bootstrap the new host -----------------------------------------------
log "bootstrapping $new_host"
scp "${ssh_opts[@]}" "$repo_root/deploy/bootstrap.sh" "$new_host:/tmp/bootstrap.sh"
ssh "${ssh_opts[@]}" "$new_host" "chmod +x /tmp/bootstrap.sh && sudo /tmp/bootstrap.sh"

# --- 2. read what we need from the main host's .env --------------------------
log "reading Postgres connection details from $main_host:$main_remote_dir/.env"
pg_bind=$(ssh "${ssh_opts[@]}" "$main_host" "grep -E '^PG_BIND=' $main_remote_dir/.env | cut -d= -f2")
app_role_password=$(ssh "${ssh_opts[@]}" "$main_host" "grep -E '^ARENA_APP_ROLE_PASSWORD=' $main_remote_dir/.env | cut -d= -f2")
db_pool_max=$(ssh "${ssh_opts[@]}" "$main_host" "grep -E '^ARENA_DB_POOL_MAX=' $main_remote_dir/.env | cut -d= -f2" || true)
db_pool_max="${db_pool_max:-20}"

if [ -z "$pg_bind" ] || [ "$pg_bind" = "127.0.0.1" ]; then
	echo "[add-worker] ERROR: $main_host's PG_BIND is empty or 127.0.0.1 — a remote" >&2
	echo "  worker can't reach Postgres over loopback. Set PG_BIND to the main" >&2
	echo "  host's private network IP in its .env, restart postgres there" >&2
	echo "  (make up), and re-run this script." >&2
	exit 1
fi
if [ -z "$app_role_password" ]; then
	echo "[add-worker] ERROR: couldn't read ARENA_APP_ROLE_PASSWORD from $main_host's .env" >&2
	exit 1
fi

# --- 3. ship the repo to the new host -----------------------------------------
log "packing and shipping repo to $new_host:$remote_dir"
tarball="$(mktemp -t tolerance-worker.XXXXXX).tar.gz"
trap 'rm -f "$tarball"' EXIT
tar_extra_args=()
tar --help 2>&1 | grep -q -- '--no-xattrs' && tar_extra_args+=(--no-xattrs)
tar --help 2>&1 | grep -q -- '--no-mac-metadata' && tar_extra_args+=(--no-mac-metadata)
COPYFILE_DISABLE=1 tar czf "$tarball" \
	${tar_extra_args[@]+"${tar_extra_args[@]}"} \
	-C "$repo_root" \
	--exclude='.git' --exclude='node_modules' --exclude='.next' \
	--exclude='.env' --exclude='.superset' --exclude='backend/.connector' \
	--exclude='._*' --exclude='.DS_Store' \
	backend deploy Makefile

ssh "${ssh_opts[@]}" "$new_host" "sudo mkdir -p $remote_dir && sudo chown \$(id -u):\$(id -g) $remote_dir"
scp "${ssh_opts[@]}" "$tarball" "$new_host:/tmp/tolerance-worker.tar.gz"
ssh "${ssh_opts[@]}" "$new_host" bash -s <<REMOTE_EXTRACT
set -euo pipefail
tar xzf /tmp/tolerance-worker.tar.gz -C $remote_dir
rm -f /tmp/tolerance-worker.tar.gz
find $remote_dir -name '._*' -type f -delete
find $remote_dir -name '.DS_Store' -type f -delete
REMOTE_EXTRACT

# --- 4. write the worker-only .env and bring it up ----------------------------
log "writing worker-only .env on $new_host and starting it"
ssh "${ssh_opts[@]}" "$new_host" bash -s <<REMOTE_UP
set -euo pipefail
cd $remote_dir

docker_gid=\$(stat -c %g /var/run/docker.sock)
# First IP on this host's default route interface — good enough for a
# single-NIC cloud VM; override WORKER_BIND by hand afterwards for
# anything more exotic.
worker_bind=\$(hostname -I 2>/dev/null | awk '{print \$1}')
[ -z "\$worker_bind" ] && worker_bind=127.0.0.1

if [ -f .env ]; then
	echo "[add-worker] .env already exists on this host, leaving it as is."
else
	cat >.env <<ENV
# Written by deploy/add-worker.sh. Worker-only host: no api/web/postgres
# here, see deploy/compose.worker.yml.
ARENA_APP_ROLE_PASSWORD=$app_role_password
MAIN_PG_HOST=$pg_bind
ARENA_DB_POOL_MAX=$db_pool_max
ARENA_WORKER_CONCURRENCY=$concurrency
DOCKER_GID=\$docker_gid
WORKER_BIND=\$worker_bind
ALLOY_MEM_LIMIT=256m
ENV
	chmod 600 .env
fi

docker build -q -t arena-proof-go:1 backend/fixtures/proofs/go-fix-retry
docker build -q -t arena-bot-runtime:1 backend/internal/games/match/runtime
docker compose -f deploy/compose.worker.yml --env-file .env up -d --build
docker compose -f deploy/compose.worker.yml --env-file .env ps
REMOTE_UP

worker_bind=$(ssh "${ssh_opts[@]}" "$new_host" "grep -E '^WORKER_BIND=' $remote_dir/.env | cut -d= -f2")
log "worker up on $new_host, metrics on $worker_bind:9091 (worker) and :9100 (node-exporter)"

# --- 5. register this worker with the main host's Prometheus -----------------
log "registering $worker_name with $main_host's Prometheus file_sd"
ssh "${ssh_opts[@]}" "$main_host" bash -s <<REMOTE_TARGETS
set -euo pipefail
cd $main_remote_dir
mkdir -p deploy/monitoring/targets
cat >deploy/monitoring/targets/worker-$worker_name.json <<JSON
[{"targets": ["$worker_bind:9091"], "labels": {"host": "$worker_name"}}]
JSON
cat >deploy/monitoring/targets/node-$worker_name.json <<JSON
[{"targets": ["$worker_bind:9100"], "labels": {"host": "$worker_name"}}]
JSON
REMOTE_TARGETS

log "done. $new_host is now a worker host (concurrency=$concurrency)."
log "Prometheus on $main_host picks up the new targets automatically (file_sd, no restart needed)."
log "Remember: restrict $main_host's 5432/3100 to the private subnet with deploy/cf-origin-lock.sh if not already done."
