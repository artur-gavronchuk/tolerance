#!/usr/bin/env bash
# Run ONCE on a fresh Ubuntu server (as root) before the first deploy.sh.
# Idempotent: safe to re-run after a package update or to pick up new
# sysctl/limits tuning.
#
# Deliberately leaves alone: existing services on other ports (e.g. a
# `danted` SOCKS proxy), ufw/firewall rules (see deploy/cf-origin-lock.sh
# for origin lock-down, which is iptables DOCKER-USER, not ufw), and SSH.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
	echo "run as root (sudo $0)" >&2
	exit 1
fi

log() { echo "[bootstrap] $*"; }

# --- packages ----------------------------------------------------------------
export DEBIAN_FRONTEND=noninteractive
log "apt-get update"
apt-get update -qq

log "installing docker.io, compose, buildx, make, git, unattended-upgrades"
apt-get install -y -qq \
	docker.io docker-compose-v2 docker-buildx \
	make git curl jq unattended-upgrades apt-listchanges

systemctl enable --now docker >/dev/null

# --- swap ----------------------------------------------------------------
# Only if there's no swap at all yet; never resize/replace an existing one.
if [ "$(swapon --show=NAME --noheadings | wc -l)" -eq 0 ] && [ ! -f /swapfile ]; then
	ram_kb=$(awk '/MemTotal/{print $2}' /proc/meminfo)
	ram_gb=$((ram_kb / 1024 / 1024))
	if [ "$ram_gb" -lt 8 ]; then
		swap_size="4G"
	else
		swap_size="8G"
	fi
	log "no swap found, creating ${swap_size} swapfile"
	fallocate -l "$swap_size" /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=$((${swap_size%G} * 1024))
	chmod 600 /swapfile
	mkswap /swapfile
	swapon /swapfile
	grep -q '^/swapfile ' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
else
	log "swap already present, leaving it alone"
fi

# --- docker daemon: log rotation + live-restore ------------------------------
mkdir -p /etc/docker
docker_daemon_json=/etc/docker/daemon.json
desired_json=$(cat <<'EOF'
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "5" },
  "live-restore": true
}
EOF
)
if [ ! -f "$docker_daemon_json" ] || ! diff -q <(echo "$desired_json") "$docker_daemon_json" >/dev/null 2>&1; then
	log "writing $docker_daemon_json"
	echo "$desired_json" >"$docker_daemon_json"
	systemctl restart docker
else
	log "$docker_daemon_json already up to date"
fi

# --- sysctl tuning for a public-facing box under load ------------------------
sysctl_file=/etc/sysctl.d/99-tolerance.conf
cat >"$sysctl_file" <<'EOF'
# Written by deploy/bootstrap.sh. Tuning for a public HTTP server expecting
# a large burst of short-lived connections plus thousands of long-poll
# connector connections.
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_tw_reuse = 1
fs.file-max = 2097152
# Prefer reclaiming caches over swapping under memory pressure; we still
# want swap as a safety net (see swapfile above), not a working set.
vm.swappiness = 10
EOF
sysctl --system >/dev/null
log "applied $sysctl_file"

# --- raise nofile limits for the docker daemon (containers inherit it) ------
mkdir -p /etc/systemd/system/docker.service.d
cat >/etc/systemd/system/docker.service.d/limits.conf <<'EOF'
[Service]
LimitNOFILE=1048576
LimitNPROC=infinity
EOF
systemctl daemon-reload
systemctl restart docker
log "raised docker.service nofile/nproc limits"

# --- unattended-upgrades ------------------------------------------------------
if [ -f /etc/apt/apt.conf.d/20auto-upgrades ]; then
	log "unattended-upgrades already configured"
else
	cat >/etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
EOF
	log "enabled unattended-upgrades"
fi
systemctl enable --now unattended-upgrades >/dev/null 2>&1 || true

# --- free :80/:443 from any other webserver ----------------------------------
for svc in nginx apache2; do
	if systemctl is-active --quiet "$svc" 2>/dev/null; then
		log "stopping and disabling $svc (Caddy needs :80/:443)"
		systemctl disable --now "$svc"
	fi
done

log "bootstrap done. Existing unrelated services (e.g. danted) were left untouched."
log "Next: from your dev machine, deploy/deploy.sh <ssh-host>"
