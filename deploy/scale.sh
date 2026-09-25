#!/usr/bin/env bash
# Change replica counts / worker concurrency on a running server without a
# rebuild or downtime for the other services.
#
# Usage: deploy/scale.sh <ssh-host> [workers=N] [web=N] [api=N] [concurrency=N]
#   workers=N       WORKER_REPLICAS (extra worker containers on this host)
#   web=N           WEB_REPLICAS
#   api=N           API_REPLICAS
#   concurrency=N   ARENA_WORKER_CONCURRENCY (parallel sandbox runs per
#                   worker container)
# Any key can be omitted; only the ones given are changed. Also callable as
# `make scale HOST=<ssh-host> ARGS="workers=3"`.
set -euo pipefail

host="${1:?usage: deploy/scale.sh <ssh-host> [workers=N] [web=N] [api=N] [concurrency=N]}"
shift
remote_dir=/opt/tolerance
ssh_opts=(-o IPQoS=none -o ServerAliveInterval=15)

declare -A KEY_FOR=([workers]=WORKER_REPLICAS [web]=WEB_REPLICAS [api]=API_REPLICAS [concurrency]=ARENA_WORKER_CONCURRENCY)
declare -A SCALE_FOR=([workers]=worker [web]=web [api]=api)

env_updates=""
scale_flags=""
for arg in "$@"; do
	key=${arg%%=*}
	val=${arg#*=}
	env_key="${KEY_FOR[$key]:-}"
	if [ -z "$env_key" ]; then
		echo "unknown key '$key' (expected workers|web|api|concurrency)" >&2
		exit 1
	fi
	case "$val" in
	'' | *[!0-9]*)
		echo "value for $key must be a positive integer, got '$val'" >&2
		exit 1
		;;
	esac
	env_updates="$env_updates $env_key=$val"
	svc="${SCALE_FOR[$key]:-}"
	[ -n "$svc" ] && scale_flags="$scale_flags --scale $svc=$val"
done
env_updates="${env_updates# }"
scale_flags="${scale_flags# }"

if [ -z "$env_updates" ]; then
	echo "nothing to change; usage: deploy/scale.sh <ssh-host> [workers=N] [web=N] [api=N] [concurrency=N]" >&2
	exit 1
fi

# Both remote scripts are sent as plain stdin via an UNQUOTED heredoc, so
# $env_updates/$scale_flags are substituted locally into the script text
# before it's shipped — this sidesteps ssh's habit of rejoining/re-splitting
# separate command-line arguments on the remote end, which would otherwise
# mangle values containing spaces.
echo "[scale] updating .env on $host:$env_updates"
# shellcheck disable=SC2087
ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE
set -euo pipefail
cd $remote_dir
for kv in $env_updates; do
	key=\${kv%%=*}
	val=\${kv#*=}
	if grep -q "^\${key}=" .env; then
		sed -i "s/^\${key}=.*/\${key}=\${val}/" .env
	else
		echo "\${key}=\${val}" >>.env
	fi
done
REMOTE

# --scale only actually changes anything for services that don't publish a
# fixed host port and have no deploy.replicas conflict — that's why
# deploy/compose.prod.yml removes api/web's host ports. --no-build keeps
# this fast: no image rebuild, no downtime for postgres/caddy/etc. A
# concurrency-only change has no --scale flag, so recreate just the worker
# to pick up the new env (still no rebuild).
if [ -n "$scale_flags" ]; then
	echo "[scale] applying with docker compose up -d --no-build $scale_flags"
else
	echo "[scale] recreating worker only, to apply new ARENA_WORKER_CONCURRENCY"
fi
# shellcheck disable=SC2087
ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE
set -euo pipefail
cd $remote_dir
set -a
. ./.env
set +a
profiles="--profile prod"
[ "\${ARENA_MONITORING:-false}" = "true" ] && profiles="\$profiles --profile monitoring"
compose_files="-f docker-compose.yml -f deploy/compose.prod.yml"
if [ -n "$scale_flags" ]; then
	docker compose \$compose_files \$profiles up -d --no-build $scale_flags
else
	docker compose \$compose_files \$profiles up -d --no-build --no-deps worker
fi
REMOTE

echo
echo "[scale] current state:"
# shellcheck disable=SC2029
ssh "${ssh_opts[@]}" "$host" "cd $remote_dir && docker compose ps"
echo
echo "[scale] current load:"
ssh "${ssh_opts[@]}" "$host" "uptime; free -h | head -2"
