#!/usr/bin/env bash
# shellcheck disable=SC2087,SC2029
# Quick health check of a running server over ssh.
# Usage: deploy/status.sh <ssh-host>
set -euo pipefail

host="${1:?usage: deploy/status.sh <ssh-host>}"
remote_dir=/opt/tolerance
ssh_opts=(-o IPQoS=none -o ServerAliveInterval=15)

ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE
set -euo pipefail
cd $remote_dir

echo "== docker compose ps =="
docker compose ps

echo
echo "== memory =="
free -h

echo
echo "== disk =="
df -h /

echo
echo "== uptime / load =="
uptime

echo
echo "== last 20 api log lines with errors =="
docker compose logs --tail=500 api 2>/dev/null | grep -iE "error|panic|fatal" | tail -20 || echo "(none found in the last 500 lines)"
REMOTE
