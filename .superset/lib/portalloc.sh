#!/usr/bin/env bash
# Shared per-workspace port allocation, compatible with Superset's own
# convention (https://docs.superset.sh/ports): a global, cross-project
# registry at ~/.superset/port-allocations.json keyed by worktree path, so
# concurrent workspaces (in this repo or any other adopting the same
# convention) never collide. Sourced by setup.sh and teardown.sh; never run
# directly.

ARENA_PORT_ALLOC_FILE="$HOME/.superset/port-allocations.json"
ARENA_PORT_ALLOC_LOCK="$HOME/.superset/port-allocations.lock"
ARENA_PORT_START=3000
ARENA_PORT_RANGE=20

# Ports the OS or common dev tooling reserve/refuse to bind, so a workspace
# never gets handed a base whose window contains one of these.
ARENA_RESERVED_PORTS="3659 4045 5000 5060 5061 6000 6566 6665 6666 6667 6668 6669 6697 7000"

arena_port_base_is_safe() {
  local base=$1 range=$2 reserved
  for reserved in $ARENA_RESERVED_PORTS; do
    if [ "$reserved" -ge "$base" ] && [ "$reserved" -lt "$((base + range))" ]; then
      return 1
    fi
  done
  return 0
}

# True if nothing is currently listening on 127.0.0.1:<base + offset> for
# every offset given (e.g. `arena_port_base_free 3000 0 1 2`). Guards against
# ports held by things outside this registry (another app, a manually
# started stack). Best-effort: a race after this check is still possible.
arena_port_base_free() {
  local base=$1 off port
  shift
  for off in "$@"; do
    port=$((base + off))
    if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
      exec 3<&- 2>/dev/null
      exec 3>&- 2>/dev/null
      return 1
    fi
  done
  return 0
}

arena_acquire_port_lock() {
  local timeout_seconds=30 stale_seconds=300 waited=0

  while ! mkdir "$ARENA_PORT_ALLOC_LOCK" 2>/dev/null; do
    local cleaned=false pid_file="$ARENA_PORT_ALLOC_LOCK/pid" lock_pid=""
    if [ -f "$pid_file" ]; then
      lock_pid="$(cat "$pid_file" 2>/dev/null || true)"
      if [ -n "$lock_pid" ] && ! kill -0 "$lock_pid" 2>/dev/null; then
        rm -rf "$ARENA_PORT_ALLOC_LOCK" 2>/dev/null || true
        cleaned=true
      fi
    fi
    if [ "$cleaned" = false ]; then
      local mtime
      mtime=$(stat -f %m "$ARENA_PORT_ALLOC_LOCK" 2>/dev/null || stat -c %Y "$ARENA_PORT_ALLOC_LOCK" 2>/dev/null || true)
      if [ -n "$mtime" ] && [ $(( $(date +%s) - mtime )) -ge "$stale_seconds" ]; then
        rm -rf "$ARENA_PORT_ALLOC_LOCK" 2>/dev/null || true
        cleaned=true
      fi
    fi
    [ "$cleaned" = true ] && continue
    if [ "$waited" -ge "$timeout_seconds" ]; then
      echo "Timed out waiting for port allocation lock: $ARENA_PORT_ALLOC_LOCK" >&2
      return 1
    fi
    sleep 1
    waited=$((waited + 1))
  done

  printf '%s\n' "$$" >"$ARENA_PORT_ALLOC_LOCK/pid" 2>/dev/null || true
  return 0
}

arena_release_port_lock() {
  rm -rf "$ARENA_PORT_ALLOC_LOCK" 2>/dev/null || true
}

# Allocates (or reuses) a port base for $PWD and prints it on stdout.
# Any extra args are offsets from the base that must be actually free on the
# host right now (e.g. `arena_allocate_port_base 0 1 2` for a 3-port stack).
arena_allocate_port_base() {
  mkdir -p "$HOME/.superset"
  [ -f "$ARENA_PORT_ALLOC_FILE" ] || echo '{}' >"$ARENA_PORT_ALLOC_FILE"

  arena_acquire_port_lock || return 1

  local key="$PWD" existing
  existing=$(jq -r --arg k "$key" '.[$k] // empty' "$ARENA_PORT_ALLOC_FILE" 2>/dev/null) || {
    echo "Failed to read $ARENA_PORT_ALLOC_FILE" >&2
    arena_release_port_lock
    return 1
  }

  if [ -n "$existing" ] && arena_port_base_is_safe "$existing" "$ARENA_PORT_RANGE" && arena_port_base_free "$existing" "$@"; then
    printf '%s\n' "$existing"
    arena_release_port_lock
    return 0
  fi

  local used candidate="$ARENA_PORT_START" tmp_file="${ARENA_PORT_ALLOC_FILE}.tmp.$$"
  used=$(jq -r '[.[]] | sort | .[]' "$ARENA_PORT_ALLOC_FILE" 2>/dev/null) || {
    echo "Failed to parse $ARENA_PORT_ALLOC_FILE" >&2
    arena_release_port_lock
    return 1
  }
  while echo "$used" | grep -qx "$candidate" 2>/dev/null \
    || ! arena_port_base_is_safe "$candidate" "$ARENA_PORT_RANGE" \
    || ! arena_port_base_free "$candidate" "$@"; do
    candidate=$((candidate + ARENA_PORT_RANGE))
  done

  jq --arg k "$key" --argjson v "$candidate" '. + {($k): $v}' "$ARENA_PORT_ALLOC_FILE" >"$tmp_file" \
    && mv "$tmp_file" "$ARENA_PORT_ALLOC_FILE" || {
    echo "Failed to persist port allocation" >&2
    rm -f "$tmp_file"
    arena_release_port_lock
    return 1
  }

  printf '%s\n' "$candidate"
  arena_release_port_lock
  return 0
}

arena_release_port_base() {
  [ -f "$ARENA_PORT_ALLOC_FILE" ] || return 0
  arena_acquire_port_lock || return 1

  local key="$PWD" tmp_file="${ARENA_PORT_ALLOC_FILE}.tmp.$$"
  jq --arg k "$key" 'del(.[$k])' "$ARENA_PORT_ALLOC_FILE" >"$tmp_file" \
    && mv "$tmp_file" "$ARENA_PORT_ALLOC_FILE" || {
    echo "Failed to release port allocation" >&2
    rm -f "$tmp_file"
    arena_release_port_lock
    return 1
  }

  arena_release_port_lock
  return 0
}
