#!/usr/bin/env bash
# Runs ON the production server as the `deploy` user (never locally). Called
# over SSH by .github/workflows/deploy.yml, one short-lived subcommand per
# SSH connection so a dropped connection never leaves a long-running step
# with nothing watching it — every subcommand below either finishes fast or
# is safe to re-run (idempotent) if the connection that started it dropped.
#
# Usage: deploy/release.sh <subcommand> [args...]
#
#   pull <backend|web> <tag>       Pull image(s) from GHCR for this component.
#                                   backend also opportunistically pulls the
#                                   proof-go/bot-runtime images at the same
#                                   tag and re-tags them to the fixed local
#                                   names internal/proofs and games/match
#                                   expect (arena-proof-go:1,
#                                   arena-bot-runtime:1) — see the module
#                                   comment in cmd/api/main.go. Missing at
#                                   this tag (unchanged this release) is not
#                                   an error: the previously tagged image is
#                                   left as-is.
#   migrate <tag>                  pg_dump into the backups volume, then run
#                                   the one-shot `migrate` service with the
#                                   backend image at <tag> — WITHOUT changing
#                                   API_TAG in .env, so the running api/worker
#                                   stay on the old image throughout (this is
#                                   exactly why migrations must be backward
#                                   compatible; see CLAUDE.md). Non-zero on
#                                   failure; api/worker are never touched.
#   canary-start <backend|web> <tag>
#                                   Bring up the *-canary sibling(s) at <tag>
#                                   and give them CANARY_WEIGHT% of traffic
#                                   (env, default 10). Safe to re-run.
#   canary-check <backend|web>     One evaluation window. Exit 0 = healthy,
#                                   non-zero = unhealthy (workflow aborts).
#   deploy-direct <backend|web> <tag>
#                                   No canary phase at all: set the tag and
#                                   `up -d --no-build`, wait healthy, record
#                                   in .deployed/. Used when the workflow was
#                                   dispatched with canary=false — accept a
#                                   few seconds of restart for that
#                                   component instead of a traffic-shifted
#                                   rollout (see .github/workflows/deploy.yml).
#   promote <backend|web>          Send 100% to canary, recreate stable on
#                                   the canary's tag, wait healthy, send
#                                   100% back to (new) stable, remove
#                                   canary, record the tag in .deployed/.
#   abort <backend|web>            Weight back to 0, remove canary, leave
#                                   stable exactly as it was.
#   rollback <backend|web>         Redeploy the previous tag from
#                                   .deployed/<component>.prev directly, no
#                                   canary phase. Does NOT revert migrations
#                                   (see CLAUDE.md: expand/contract only).
#   apply-config                   Reload Caddy (always) and Prometheus/
#                                   Grafana (only if their config actually
#                                   changed) after deploy/ship-config.sh has
#                                   synced non-image files onto the server.
#   status                         Human-readable summary of what's deployed.
#
# Env vars this script reads (never persisted to .env unless noted):
#   GHCR_TOKEN, GHCR_USER   Optional; only needed if GHCR denies an
#                           anonymous pull (packages are public by default
#                           for this repo). GHCR_USER defaults to "token".
#   CANARY_WEIGHT           Percent of traffic to canary once canary-start
#                           has run. Default 10.
#   CANARY_QUERY_WINDOW     Prometheus rate() window for canary-check.
#                           Default 5m.
#   CANARY_MAX_5XX_FLOOR, CANARY_5XX_MULT
#                           canary 5xx% must stay under
#                           max(CANARY_MAX_5XX_FLOOR, stable_5xx * CANARY_5XX_MULT).
#                           Defaults 1 and 2 (i.e. "< max(1%, 2x stable)").
#   CANARY_P95_MULT, CANARY_P95_MARGIN_MS
#                           canary p95 must stay under
#                           stable_p95 * CANARY_P95_MULT + CANARY_P95_MARGIN_MS.
#                           Defaults 1.5 and 200.
#   CANARY_INFRA_MULT, CANARY_INFRA_FLOOR
#                           canary infra_error rate (/s) must stay under
#                           max(CANARY_INFRA_FLOOR, stable_rate * CANARY_INFRA_MULT).
#                           Defaults 2 and 0.05.
set -euo pipefail

REMOTE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REMOTE_DIR"

BACKEND_IMAGE=ghcr.io/artur-gavronchuk/tolerance-backend
WEB_IMAGE=ghcr.io/artur-gavronchuk/tolerance-web
PROOF_IMAGE=ghcr.io/artur-gavronchuk/tolerance-proof-go
BOT_IMAGE=ghcr.io/artur-gavronchuk/tolerance-bot-runtime

log() { echo "[release] $*"; }
die() {
	echo "[release] ERROR: $*" >&2
	exit 1
}

[ -f .env ] || die ".env not found in $REMOTE_DIR — this doesn't look like a bootstrapped server"

# --- compose plumbing ---------------------------------------------------------

# Every invocation includes --profile canary: `up`/`create` with an explicit
# service name work regardless of profile per Compose semantics, and
# ps/exec/down need the flag to see/act on canary services at all once they
# exist. Including it unconditionally is a no-op when canary isn't up.
dc() {
	set -a
	# shellcheck disable=SC1091
	. ./.env
	set +a
	local profiles=(--profile prod --profile canary)
	[ "${ARENA_MONITORING:-false}" = "true" ] && profiles+=(--profile monitoring)
	docker compose -f docker-compose.yml -f deploy/compose.prod.yml "${profiles[@]}" "$@"
}

env_get() { grep -E "^$1=" .env 2>/dev/null | tail -1 | cut -d= -f2-; }

# env_set KEY VALUE — updates .env in place, appending if the key is absent.
# The only writer of .env this script (or any CI-driven script) should use:
# it only ever replaces a single KEY=... line, never the whole file, per
# CLAUDE.md's "CI must never overwrite .env" rule.
env_set() {
	local key=$1 val=$2
	if grep -q "^${key}=" .env; then
		sed -i.bak "s|^${key}=.*|${key}=${val}|" .env && rm -f .env.bak
	else
		echo "${key}=${val}" >>.env
	fi
}

require_component() {
	case "$1" in
	backend | web) ;;
	*) die "component must be backend or web, got '$1'" ;;
	esac
}

# --- pull ----------------------------------------------------------------------

ghcr_login_if_needed() {
	[ -n "${GHCR_TOKEN:-}" ] || return 1
	echo "$GHCR_TOKEN" | docker login ghcr.io -u "${GHCR_USER:-token}" --password-stdin >/dev/null 2>&1
}

pull_required() {
	local ref=$1
	if docker pull "$ref"; then
		return 0
	fi
	log "anonymous pull of $ref failed, trying GHCR_TOKEN if set"
	if ghcr_login_if_needed && docker pull "$ref"; then
		return 0
	fi
	die "could not pull $ref (set GHCR_TOKEN/GHCR_USER if the package isn't public)"
}

# pull_optional REF LOCAL_TAG — used for proof-go/bot-runtime, which are only
# rebuilt when their own paths change (see ci path filters), so most backend
# releases won't have a new image at this exact tag. Missing is not fatal:
# the existing local tag (from a previous release, or `make up`'s local
# build) is left alone and the worker keeps using it.
pull_optional() {
	local ref=$1 local_tag=$2
	if docker pull "$ref" >/tmp/release-pull-optional.log 2>&1; then
		docker tag "$ref" "$local_tag"
		log "tagged $local_tag <- $ref"
	else
		log "no $ref at this tag (unchanged this release); keeping existing $local_tag"
	fi
}

cmd_pull() {
	local component=$1 tag=${2:?usage: release.sh pull <backend|web> <tag>}
	require_component "$component"
	case "$component" in
	backend)
		pull_required "$BACKEND_IMAGE:$tag"
		pull_optional "$PROOF_IMAGE:$tag" arena-proof-go:1
		pull_optional "$BOT_IMAGE:$tag" arena-bot-runtime:1
		;;
	web)
		pull_required "$WEB_IMAGE:$tag"
		;;
	esac
	log "pull ok: $component @ $tag"
}

# --- migrate ---------------------------------------------------------------

backup_pre_deploy() {
	local tag=$1
	local project network volume ts dumpfile
	project=$(dc config --format json | jq -r .name)
	network="${project}_default"
	volume=$(docker volume ls -q --filter "name=${project}_arena-backups" | head -1)
	[ -n "$volume" ] || die "arena-backups volume not found (is the stack up?)"
	ts=$(date -u +%Y%m%dT%H%M%SZ)
	dumpfile="pre-deploy-${tag}-${ts}.dump"
	log "pre-deploy backup: $dumpfile"
	docker run --rm --network "$network" -v "${volume}:/backups" \
		-e PGPASSWORD="$(env_get POSTGRES_PASSWORD)" \
		postgres:16-alpine \
		pg_dump -h postgres -U arena_migrate -d arena -Fc -f "/backups/${dumpfile}"
	# Keep the last 10 pre-deploy dumps (backend/dev/backup.sh keeps its own
	# daily dumps separately, no overlap in filename prefix).
	docker run --rm -v "${volume}:/backups" alpine:3.22 \
		sh -c 'ls -1t /backups/pre-deploy-*.dump 2>/dev/null | tail -n +11 | xargs -r rm --'
	log "pre-deploy backup ok: $dumpfile (kept, last 10 retained)"
}

cmd_migrate() {
	local tag=${1:?usage: release.sh migrate <tag>}
	backup_pre_deploy "$tag"
	log "running migrate at tag $tag (api/worker stay on the current tag until promote)"
	if ! API_TAG="$tag" dc run --rm migrate; then
		die "migration failed at tag $tag — stable api/worker are untouched; see the migrate logs above. Restore from the pre-deploy dump if the failure left the schema half-applied (README.md, 'Восстановление из дампа')."
	fi
	log "migrate ok"
}

# --- canary weight / Caddy ---------------------------------------------------

render_upstream() {
	local component=$1 weight=$2 port stable canary file
	case "$component" in
	backend)
		file=deploy/caddy/upstreams/api.caddy
		stable=api
		canary=api-canary
		port=8080
		;;
	web)
		file=deploy/caddy/upstreams/web.caddy
		stable=web
		canary=web-canary
		port=3000
		;;
	esac
	if [ "$weight" -le 0 ]; then
		cat >"$file" <<CFG
dynamic a $stable $port {
	refresh 5s
}
lb_policy least_conn
CFG
	elif [ "$weight" -ge 100 ]; then
		cat >"$file" <<CFG
dynamic a $canary $port {
	refresh 5s
}
lb_policy least_conn
CFG
	else
		local stable_w=$((100 - weight))
		cat >"$file" <<CFG
to $stable:$port $canary:$port
lb_policy weighted_round_robin $stable_w $weight
CFG
	fi
	log "upstream $component -> weight $weight% canary ($file)"
}

reload_caddy() {
	dc exec -T caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
	dc exec -T caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
}

# apply-config: run once after deploy/ship-config.sh has synced
# docker-compose.yml/Caddyfile/deploy/**/backend/dev/backup.sh onto the
# server (see .github/workflows/deploy.yml). Always reloads Caddy (cheap,
# and Caddy no-ops a reload of unchanged config); Prometheus/Grafana are
# only reloaded/restarted when their config actually changed, tracked by a
# checksum in .deployed/monitoring.sha256 so an unrelated release doesn't
# pay for a Grafana restart every time.
cmd_apply_config() {
	log "validating and reloading Caddy"
	reload_caddy
	if [ "$(env_get ARENA_MONITORING)" = "true" ]; then
		mkdir -p .deployed
		local sum_file=".deployed/monitoring.sha256" old_sum new_sum
		new_sum=$(cat deploy/monitoring/prometheus.yml \
			deploy/monitoring/grafana/provisioning/alerting/rules.yaml \
			deploy/monitoring/grafana/dashboards/*.json 2>/dev/null | sha256sum | cut -d' ' -f1)
		old_sum=$(cat "$sum_file" 2>/dev/null || true)
		if [ "$new_sum" != "$old_sum" ]; then
			log "monitoring config changed: reloading Prometheus, restarting Grafana"
			dc exec -T prometheus wget -q -O - --post-data='' http://localhost:9090/-/reload \
				|| dc restart prometheus
			dc restart grafana
			echo "$new_sum" >"$sum_file"
		else
			log "monitoring config unchanged, leaving Prometheus/Grafana as is"
		fi
	fi
	log "apply-config ok"
}

wait_running() {
	local svc tries state
	for svc in "$@"; do
		tries=0
		while true; do
			state=$(dc ps --format '{{.Service}} {{.State}}' | awk -v s="$svc" '$1==s{print $2}')
			[ "$state" = "running" ] && break
			tries=$((tries + 1))
			[ "$tries" -ge 30 ] && die "$svc did not reach running state in time (last state: ${state:-absent})"
			sleep 2
		done
		log "$svc running"
	done
}

# --- canary-start ------------------------------------------------------------

cmd_canary_start() {
	local component=${1:?usage: release.sh canary-start <backend|web> <tag>}
	local tag=${2:?usage: release.sh canary-start <backend|web> <tag>}
	require_component "$component"
	local weight="${CANARY_WEIGHT:-10}"
	case "$component" in
	backend)
		env_set API_CANARY_TAG "$tag"
		API_CANARY_TAG="$tag" dc up -d --no-build api-canary worker-canary
		wait_running api-canary worker-canary
		render_upstream backend "$weight"
		;;
	web)
		env_set WEB_CANARY_TAG "$tag"
		WEB_CANARY_TAG="$tag" dc up -d --no-build web-canary
		wait_running web-canary
		render_upstream web "$weight"
		;;
	esac
	reload_caddy
	log "canary-start ok: $component @ $tag, weight ${weight}%"
}

# --- canary-check ------------------------------------------------------------

prom_scalar() {
	local q=$1 encoded json
	encoded=$(jq -rn --arg q "$q" '$q|@uri')
	json=$(dc exec -T prometheus wget -qO- "http://localhost:9090/api/v1/query?query=${encoded}")
	echo "$json" | jq -r '.data.result[0].value[1] // "0"'
}

gt() { awk -v a="$1" -v b="$2" 'BEGIN{exit !(a>b)}'; }

canary_check_backend() {
	local window="${CANARY_QUERY_WINDOW:-5m}"
	local stable_5xx canary_5xx stable_p95 canary_p95 stable_infra canary_infra

	stable_5xx=$(prom_scalar "(sum(rate(arena_http_requests_total{code=~\"5..\",track=\"stable\"}[$window])) or vector(0)) / clamp_min(sum(rate(arena_http_requests_total{track=\"stable\"}[$window])),1) * 100")
	canary_5xx=$(prom_scalar "(sum(rate(arena_http_requests_total{code=~\"5..\",track=\"canary\"}[$window])) or vector(0)) / clamp_min(sum(rate(arena_http_requests_total{track=\"canary\"}[$window])),1) * 100")
	stable_p95=$(prom_scalar "histogram_quantile(0.95, sum by (le) (rate(arena_http_request_duration_seconds_bucket{track=\"stable\"}[$window])))")
	canary_p95=$(prom_scalar "histogram_quantile(0.95, sum by (le) (rate(arena_http_request_duration_seconds_bucket{track=\"canary\"}[$window])))")
	stable_infra=$(prom_scalar "sum(rate(arena_proof_verdicts_total{status=\"infra_error\",track=\"stable\"}[$window]))")
	canary_infra=$(prom_scalar "sum(rate(arena_proof_verdicts_total{status=\"infra_error\",track=\"canary\"}[$window]))")

	log "canary-check backend: stable_5xx=${stable_5xx}% canary_5xx=${canary_5xx}% stable_p95=${stable_p95}s canary_p95=${canary_p95}s stable_infra=${stable_infra}/s canary_infra=${canary_infra}/s"

	local max5xx maxp95 maxinfra
	max5xx=$(awk -v s="$stable_5xx" -v floor="${CANARY_MAX_5XX_FLOOR:-1}" -v mult="${CANARY_5XX_MULT:-2}" 'BEGIN{v=s*mult; if(v<floor)v=floor; print v}')
	maxp95=$(awk -v s="$stable_p95" -v mult="${CANARY_P95_MULT:-1.5}" -v margin="${CANARY_P95_MARGIN_MS:-200}" 'BEGIN{print s*mult + margin/1000}')
	maxinfra=$(awk -v s="$stable_infra" -v mult="${CANARY_INFRA_MULT:-2}" -v floor="${CANARY_INFRA_FLOOR:-0.05}" 'BEGIN{v=s*mult; if(v<floor)v=floor; print v}')

	local failed=0
	if gt "$canary_5xx" "$max5xx"; then
		log "FAIL: canary 5xx ${canary_5xx}% > ${max5xx}%"
		failed=1
	fi
	if gt "$canary_p95" "$maxp95"; then
		log "FAIL: canary p95 ${canary_p95}s > ${maxp95}s"
		failed=1
	fi
	if gt "$canary_infra" "$maxinfra"; then
		log "FAIL: canary infra_error rate ${canary_infra}/s > ${maxinfra}/s"
		failed=1
	fi
	[ "$failed" -eq 0 ] || return 1
	log "canary-check backend: OK"
}

# check_page PATH MARKER — fetched from inside the caddy container so it
# reaches web-canary directly, bypassing the load balancer entirely (this
# must test the canary, not whichever way the coin lands on the weighted
# split). MARKER empty skips the content check (used for /app, which is a
# client-rendered dashboard behind auth and has no fixed server-rendered
# marker text worth pinning to).
check_page() {
	local path=$1 marker=$2 body
	if ! body=$(dc exec -T caddy wget -q -O - "http://web-canary:3000${path}" 2>&1); then
		log "FAIL: web-canary${path} did not return 200"
		return 1
	fi
	if [ -n "$marker" ] && ! grep -qi "$marker" <<<"$body"; then
		log "FAIL: web-canary${path} missing expected content ('$marker')"
		return 1
	fi
	log "web-canary${path} OK"
}

canary_check_web() {
	local failed=0
	check_page / "prove it" || failed=1
	check_page /login "Sign in" || failed=1
	check_page /app "" || failed=1
	check_page /terms "Terms and fair play" || failed=1

	# Overall Caddy 5xx rate must not be rising while the canary holds
	# CANARY_WEIGHT of the traffic — web has no application metrics of its
	# own, so this is the only quantitative signal available for it. Caddy's
	# built-in `metrics` global option (see the Caddyfile) exports
	# caddy_http_requests_total.
	local window="${CANARY_QUERY_WINDOW:-5m}"
	local rate5xx
	rate5xx=$(prom_scalar "(sum(rate(caddy_http_requests_total{code=~\"5..\"}[$window])) or vector(0)) / clamp_min(sum(rate(caddy_http_requests_total[$window])),1) * 100")
	log "canary-check web: overall Caddy 5xx rate = ${rate5xx}%"
	if gt "$rate5xx" "${CANARY_MAX_5XX_FLOOR:-1}"; then
		log "FAIL: overall Caddy 5xx rate ${rate5xx}% > ${CANARY_MAX_5XX_FLOOR:-1}%"
		failed=1
	fi
	[ "$failed" -eq 0 ] || return 1
	log "canary-check web: OK"
}

cmd_canary_check() {
	local component=${1:?usage: release.sh canary-check <backend|web>}
	require_component "$component"
	case "$component" in
	backend) canary_check_backend ;;
	web) canary_check_web ;;
	esac
}

# --- promote / abort / rollback ----------------------------------------------

record_deployed() {
	local component=$1 tag=$2
	mkdir -p .deployed
	if [ -f ".deployed/$component" ]; then
		cp ".deployed/$component" ".deployed/$component.prev"
	fi
	echo "$tag" >".deployed/$component"
	post_grafana_annotation "$component" "$tag"
}

# post_grafana_annotation: a "deploy" tag on the Grafana overview dashboard's
# timeline (its annotation query is provisioned to show them — see
# deploy/monitoring/grafana/dashboards/tolerance-overview.json). Best-effort:
# monitoring off, no admin password yet, or Grafana unreachable are all
# silently skipped rather than failing the release over an annotation.
post_grafana_annotation() {
	local component=$1 tag=$2
	[ "$(env_get ARENA_MONITORING)" = "true" ] || return 0
	local pass
	pass=$(env_get GRAFANA_ADMIN_PASSWORD)
	[ -n "$pass" ] || return 0
	local body auth
	body="{\"text\":\"deploy: ${component} @ ${tag}\",\"tags\":[\"deploy\",\"${component}\"]}"
	auth=$(printf '%s' "admin:${pass}" | base64)
	dc exec -T caddy wget -q -O /dev/null \
		--header="Authorization: Basic ${auth}" \
		--header="Content-Type: application/json" \
		--post-data="${body}" \
		http://grafana:3000/api/annotations \
		|| log "grafana annotation post failed (non-fatal)"
}

cmd_deploy_direct() {
	local component=${1:?usage: release.sh deploy-direct <backend|web> <tag>}
	local tag=${2:?usage: release.sh deploy-direct <backend|web> <tag>}
	require_component "$component"
	case "$component" in
	backend)
		env_set API_TAG "$tag"
		dc up -d --no-build api worker
		wait_running api worker
		;;
	web)
		env_set WEB_TAG "$tag"
		dc up -d --no-build web
		wait_running web
		;;
	esac
	record_deployed "$component" "$tag"
	log "deploy-direct ok: $component @ $tag (canary=false path; no traffic-shifted rollout, see the workflow's own comment on this tradeoff)"
}

cmd_promote() {
	local component=${1:?usage: release.sh promote <backend|web>}
	require_component "$component"
	case "$component" in
	backend)
		local tag
		tag=$(env_get API_CANARY_TAG)
		[ -n "$tag" ] || die "no API_CANARY_TAG in .env — was canary-start run for backend?"
		log "promote backend: sending 100% to canary before recreating stable"
		render_upstream backend 100
		reload_caddy
		env_set API_TAG "$tag"
		dc up -d --no-build api worker
		wait_running api worker
		log "promote backend: stable recreated at $tag, sending 100% back to stable"
		render_upstream backend 0
		reload_caddy
		dc rm -sf api-canary worker-canary
		env_set API_CANARY_TAG ""
		record_deployed backend "$tag"
		;;
	web)
		local tag
		tag=$(env_get WEB_CANARY_TAG)
		[ -n "$tag" ] || die "no WEB_CANARY_TAG in .env — was canary-start run for web?"
		log "promote web: sending 100% to canary before recreating stable"
		render_upstream web 100
		reload_caddy
		env_set WEB_TAG "$tag"
		dc up -d --no-build web
		wait_running web
		log "promote web: stable recreated at $tag, sending 100% back to stable"
		render_upstream web 0
		reload_caddy
		dc rm -sf web-canary
		env_set WEB_CANARY_TAG ""
		record_deployed web "$tag"
		;;
	esac
	log "promote ok: $component @ $tag"
}

cmd_abort() {
	local component=${1:?usage: release.sh abort <backend|web>}
	require_component "$component"
	render_upstream "$component" 0
	reload_caddy
	case "$component" in
	backend)
		dc rm -sf api-canary worker-canary
		env_set API_CANARY_TAG ""
		;;
	web)
		dc rm -sf web-canary
		env_set WEB_CANARY_TAG ""
		;;
	esac
	log "abort ok: $component canary removed, stable untouched"
}

cmd_rollback() {
	local component=${1:?usage: release.sh rollback <backend|web>}
	require_component "$component"
	local prev
	prev=$(cat ".deployed/${component}.prev" 2>/dev/null || true)
	[ -n "$prev" ] || die "no .deployed/${component}.prev to roll back to"
	log "rolling back $component to $prev (direct, no canary; migrations are not reverted — see CLAUDE.md)"
	cmd_pull "$component" "$prev"
	case "$component" in
	backend)
		env_set API_TAG "$prev"
		dc up -d --no-build api worker
		wait_running api worker
		;;
	web)
		env_set WEB_TAG "$prev"
		dc up -d --no-build web
		wait_running web
		;;
	esac
	record_deployed "$component" "$prev"
	log "rollback ok: $component @ $prev"
}

cmd_status() {
	echo "== deployed tags (.env) =="
	grep -E '^(API_TAG|WEB_TAG|API_CANARY_TAG|WEB_CANARY_TAG)=' .env || true
	echo
	echo "== .deployed/ =="
	for f in .deployed/backend .deployed/backend.prev .deployed/web .deployed/web.prev; do
		[ -f "$f" ] && echo "$f: $(cat "$f")"
	done
	echo
	echo "== docker compose ps =="
	dc ps
	echo
	echo "== upstream weights =="
	for f in deploy/caddy/upstreams/api.caddy deploy/caddy/upstreams/web.caddy; do
		echo "-- $f --"
		cat "$f" 2>/dev/null || echo "(missing — copy from ${f}.default)"
	done
}

# --- dispatch ------------------------------------------------------------------

sub="${1:-}"
[ -n "$sub" ] || die "usage: release.sh <pull|migrate|canary-start|canary-check|promote|abort|rollback|status> [args...]"
shift || true
case "$sub" in
pull) cmd_pull "$@" ;;
migrate) cmd_migrate "$@" ;;
deploy-direct) cmd_deploy_direct "$@" ;;
apply-config) cmd_apply_config "$@" ;;
canary-start) cmd_canary_start "$@" ;;
canary-check) cmd_canary_check "$@" ;;
promote) cmd_promote "$@" ;;
abort) cmd_abort "$@" ;;
rollback) cmd_rollback "$@" ;;
status) cmd_status "$@" ;;
*) die "unknown subcommand '$sub'" ;;
esac
