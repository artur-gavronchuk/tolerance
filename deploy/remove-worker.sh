#!/usr/bin/env bash
# shellcheck disable=SC2087,SC2029
# Undo deploy/add-worker.sh: stop the worker-only stack on the given host
# and de-register it from the main host's Prometheus. Does not uninstall
# Docker or undo bootstrap.sh — the host is left otherwise as is.
#
# Usage: deploy/remove-worker.sh <worker-ssh-host> <main-ssh-host>
set -euo pipefail

worker_host="${1:?usage: deploy/remove-worker.sh <worker-ssh-host> <main-ssh-host>}"
main_host="${2:?usage: deploy/remove-worker.sh <worker-ssh-host> <main-ssh-host>}"
remote_dir=/opt/tolerance
main_remote_dir=/opt/tolerance
ssh_opts=(-o ServerAliveInterval=15)
worker_name="$(echo "$worker_host" | tr -c 'A-Za-z0-9' '-' | sed 's/^-*//;s/-*$//')"

log() { echo "[remove-worker] $*"; }

log "stopping worker stack on $worker_host"
ssh "${ssh_opts[@]}" "$worker_host" bash -s <<REMOTE
set -euo pipefail
cd $remote_dir
if [ -f .env ]; then
	docker compose -f deploy/compose.worker.yml --env-file .env down
else
	echo "[remove-worker] no .env on this host, nothing to bring down"
fi
REMOTE

log "de-registering $worker_name from $main_host's Prometheus"
ssh "${ssh_opts[@]}" "$main_host" bash -s <<REMOTE
set -euo pipefail
cd $main_remote_dir
rm -f "deploy/monitoring/targets/worker-$worker_name.json" "deploy/monitoring/targets/node-$worker_name.json"
REMOTE

log "done. $worker_host's Docker install and data are left in place; re-run add-worker.sh to bring it back."
