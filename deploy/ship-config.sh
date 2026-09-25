#!/usr/bin/env bash
# shellcheck disable=SC2087,SC2029
# Ship non-image config files to the server: docker-compose.yml, Caddyfile,
# Makefile, deploy/** (release.sh, compose.prod.yml, monitoring configs,
# the committed Caddy upstream defaults, ...) and backend/dev/backup.sh
# (mounted into the `backup` service). Never .env, never application source.
#
# Used by .github/workflows/deploy.yml on every release (cheap: a few
# hundred KB) so the server always has the release.sh/compose files the
# workflow is about to drive, even on a release that only changed backend/
# or frontend/ source. Also usable by hand, same as deploy.sh.
#
# Usage: deploy/ship-config.sh <ssh-host>
#
# Like deploy/deploy.sh's own repo shipping, this stages into a fresh
# directory and rsync --delete's it into place scoped to EXACTLY the paths
# above (an explicit rsync include/exclude filter, `--exclude='*'` last) —
# so a config file removed or renamed in git actually disappears on the
# server, without touching anything else under /opt/tolerance (application
# source, .env, .deployed/, generated Caddy upstream files, Prometheus
# file_sd targets, logs, ...).
set -euo pipefail

host="${1:?usage: deploy/ship-config.sh <ssh-host>}"
remote_dir=/opt/tolerance
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ssh_opts=(-o IPQoS=none -o ServerAliveInterval=15 -o ServerAliveCountMax=6)

log() { echo "[ship-config] $*"; }

tarball="$(mktemp -t tolerance-config.XXXXXX).tar.gz"
trap 'rm -f "$tarball"' EXIT

tar_extra_args=()
tar --help 2>&1 | grep -q -- '--no-xattrs' && tar_extra_args+=(--no-xattrs)
tar --help 2>&1 | grep -q -- '--no-mac-metadata' && tar_extra_args+=(--no-mac-metadata)

log "packing config files"
COPYFILE_DISABLE=1 tar czf "$tarball" \
	${tar_extra_args[@]+"${tar_extra_args[@]}"} \
	-C "$repo_root" \
	--exclude='._*' \
	--exclude='.DS_Store' \
	docker-compose.yml Caddyfile Makefile deploy backend/dev/backup.sh

log "uploading to $host:$remote_dir"
ssh "${ssh_opts[@]}" "$host" "mkdir -p $remote_dir $remote_dir/.config-staging"
scp "${ssh_opts[@]}" "$tarball" "$host:/tmp/tolerance-config.tar.gz"
ssh "${ssh_opts[@]}" "$host" bash -s <<REMOTE_APPLY
set -euo pipefail
rm -rf "$remote_dir/.config-staging"
mkdir -p "$remote_dir/.config-staging"
tar xzf /tmp/tolerance-config.tar.gz -C "$remote_dir/.config-staging"
rm -f /tmp/tolerance-config.tar.gz
find "$remote_dir/.config-staging" -name '._*' -type f -delete
find "$remote_dir/.config-staging" -name '.DS_Store' -type f -delete
# rsync filter rules are first-match-wins, so the specific excludes for
# generated state INSIDE deploy/ must come before the broad
# --include='/deploy/***' that would otherwise shadow them. -c/--checksum:
# see deploy.sh's own rsync call for why size+mtime isn't reliable here.
rsync -ac --delete \
	--exclude='/deploy/monitoring/targets/*.json' \
	--exclude='/deploy/caddy/upstreams/*.caddy' \
	--include='/docker-compose.yml' \
	--include='/Caddyfile' \
	--include='/Makefile' \
	--include='/backend/' \
	--include='/backend/dev/' \
	--include='/backend/dev/backup.sh' \
	--include='/deploy/***' \
	--exclude='/.env' \
	--exclude='/.deployed/' \
	--exclude='/deploy.log' \
	--exclude='/deploy.pid' \
	--exclude='/deploy.exit' \
	--exclude='/.deploy-run.sh' \
	--exclude='*' \
	"$remote_dir/.config-staging/" "$remote_dir/"
rm -rf "$remote_dir/.config-staging"
# Seed the canary-upstream files if this is the very first config ship on
# this host (deploy.sh does the same on first bootstrap; harmless if they
# already exist).
mkdir -p "$remote_dir/.deployed"
for name in api web; do
	f="$remote_dir/deploy/caddy/upstreams/\$name.caddy"
	[ -f "\$f" ] || cp "\$f.default" "\$f"
done
REMOTE_APPLY

log "config shipped"
