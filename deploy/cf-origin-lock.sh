#!/usr/bin/env bash
# Restrict the published Docker ports to trusted sources, via the
# DOCKER-USER iptables chain (ufw does not see Docker-published ports at
# all, since Docker inserts its own rules ahead of ufw's).
#
#   deploy/cf-origin-lock.sh on   — 80/443 reachable only from Cloudflare's
#                                   edge IPs (fetched live, falls back to a
#                                   built-in static list); 5432/3100
#                                   (Postgres/Loki), if bound to a private
#                                   IP, reachable only from PG_ALLOW_CIDR.
#   deploy/cf-origin-lock.sh off  — removes exactly those rules, cleanly.
#
# Never touches SSH (22), the existing danted SOCKS proxy (1080), or
# anything else. Idempotent: safe to run "on" repeatedly (e.g. from a timer
# that refreshes Cloudflare's IP list), and safe to run "off" when already
# off. Turn this on only AFTER the Cloudflare orange cloud is actually
# proxying tolerance.cc — otherwise you lock yourself out of your own site.
set -euo pipefail

CF_CHAIN="TOLERANCE-CF-LOCK"
PRIV_CHAIN="TOLERANCE-PRIVATE-LOCK"
SYSTEMD_UNIT=/etc/systemd/system/tolerance-cf-lock.service
ENV_FILE="${TOLERANCE_ENV_FILE:-/opt/tolerance/.env}"
SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

# Fallback list as of 2026-09-25 (https://www.cloudflare.com/ips-v4,
# https://www.cloudflare.com/ips-v6), used only if the live fetch fails.
STATIC_V4=(
	173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22
	141.101.64.0/18 108.162.192.0/18 190.93.240.0/20 188.114.96.0/20
	197.234.240.0/22 198.41.128.0/17 162.158.0.0/15 104.16.0.0/13
	104.24.0.0/14 172.64.0.0/13 131.0.72.0/22
)
STATIC_V6=(
	2400:cb00::/32 2606:4700::/32 2803:f800::/32 2405:b500::/32
	2405:8100::/32 2a06:98c0::/29 2c0f:f248::/32
)

log() { echo "[cf-origin-lock] $*"; }

fetch_or_fallback() {
	# $1 = URL, remaining args = fallback list. Prints one CIDR per line.
	local url=$1
	shift
	local fetched
	if fetched=$(curl -fsSL -m 10 "$url" 2>/dev/null) && [ -n "$fetched" ]; then
		echo "$fetched"
	else
		log "warning: could not fetch $url, using built-in fallback list"
		printf '%s\n' "$@"
	fi
}

ensure_chain() {
	local table=$1 chain=$2
	"$table" -N "$chain" 2>/dev/null || true
}

remove_jump() {
	# Remove every DOCKER-USER rule that jumps to $2, from table $1.
	local table=$1 chain=$2
	while $table -C DOCKER-USER -p tcp --dport "$3" -j "$chain" 2>/dev/null; do
		$table -D DOCKER-USER -p tcp --dport "$3" -j "$chain"
	done
}

flush_and_delete_chain() {
	local table=$1 chain=$2
	if $table -L "$chain" >/dev/null 2>&1; then
		$table -F "$chain"
		$table -X "$chain"
	fi
}

apply_cf_lock() {
	local table=$1 ip_family=$2
	shift 2
	local ranges=("$@")

	ensure_chain "$table" "$CF_CHAIN"
	$table -F "$CF_CHAIN"
	for cidr in "${ranges[@]}"; do
		[ -n "$cidr" ] || continue
		$table -A "$CF_CHAIN" -s "$cidr" -j RETURN
	done
	$table -A "$CF_CHAIN" -j DROP

	for port in 80 443; do
		remove_jump "$table" "$CF_CHAIN" "$port"
		$table -I DOCKER-USER 1 -p tcp --dport "$port" -j "$CF_CHAIN"
	done
	log "$ip_family: 80/443 locked to ${#ranges[@]} Cloudflare ranges"
}

apply_private_lock() {
	# Restrict Postgres/Loki to PG_ALLOW_CIDR when they're bound to a
	# private IP (i.e. remote sandbox workers are expected). No-op (and
	# cleans up any previous rule) when PG_BIND is 127.0.0.1 or
	# PG_ALLOW_CIDR is unset — those ports are then unreachable from the
	# network anyway.
	local table=$1
	local pg_bind pg_cidr
	pg_bind=$(grep -E '^PG_BIND=' "$ENV_FILE" 2>/dev/null | cut -d= -f2 || true)
	pg_cidr=$(grep -E '^PG_ALLOW_CIDR=' "$ENV_FILE" 2>/dev/null | cut -d= -f2 || true)

	if [ -n "$pg_bind" ] && [ "$pg_bind" != "127.0.0.1" ] && [ -n "$pg_cidr" ]; then
		ensure_chain "$table" "$PRIV_CHAIN"
		$table -F "$PRIV_CHAIN"
		$table -A "$PRIV_CHAIN" -s "$pg_cidr" -j RETURN
		$table -A "$PRIV_CHAIN" -j DROP
		for port in 5432 3100; do
			remove_jump "$table" "$PRIV_CHAIN" "$port"
			$table -I DOCKER-USER 1 -p tcp --dport "$port" -j "$PRIV_CHAIN"
		done
		log "postgres/loki (5432/3100) locked to $pg_cidr"
	else
		for port in 5432 3100; do
			remove_jump "$table" "$PRIV_CHAIN" "$port"
		done
		flush_and_delete_chain "$table" "$PRIV_CHAIN"
	fi
}

remove_all() {
	local table=$1
	for port in 80 443; do
		remove_jump "$table" "$CF_CHAIN" "$port"
	done
	for port in 5432 3100; do
		remove_jump "$table" "$PRIV_CHAIN" "$port"
	done
	flush_and_delete_chain "$table" "$CF_CHAIN"
	flush_and_delete_chain "$table" "$PRIV_CHAIN"
}

install_systemd_unit() {
	cat >"$SYSTEMD_UNIT" <<EOF
[Unit]
Description=Re-apply tolerance Cloudflare origin lock (DOCKER-USER)
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=$SELF on

[Install]
WantedBy=multi-user.target
EOF
	systemctl daemon-reload
	systemctl enable tolerance-cf-lock.service >/dev/null
}

action="${1:?usage: cf-origin-lock.sh on|off}"

if [ "$(id -u)" -ne 0 ]; then
	echo "run as root (sudo $0 $action)" >&2
	exit 1
fi

case "$action" in
on)
	mapfile -t v4 < <(fetch_or_fallback https://www.cloudflare.com/ips-v4 "${STATIC_V4[@]}")
	mapfile -t v6 < <(fetch_or_fallback https://www.cloudflare.com/ips-v6 "${STATIC_V6[@]}")
	apply_cf_lock iptables ipv4 "${v4[@]}"
	apply_private_lock iptables
	if command -v ip6tables >/dev/null; then
		apply_cf_lock ip6tables ipv6 "${v6[@]}"
		apply_private_lock ip6tables
	fi
	install_systemd_unit
	log "origin lock ON. SSH (22) and any other port (e.g. danted on 1080) are untouched."
	;;
off)
	remove_all iptables
	command -v ip6tables >/dev/null && remove_all ip6tables
	if [ -f "$SYSTEMD_UNIT" ]; then
		systemctl disable tolerance-cf-lock.service >/dev/null 2>&1 || true
		rm -f "$SYSTEMD_UNIT"
		systemctl daemon-reload
	fi
	log "origin lock OFF, rules removed."
	;;
*)
	echo "usage: cf-origin-lock.sh on|off" >&2
	exit 1
	;;
esac
