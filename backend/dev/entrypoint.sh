#!/bin/sh
set -eu

# Usage: entrypoint.sh [migrate|api|worker]
#
# migrate:      run cmd/migrate once and exit. The compose stack runs this
#               as a one-shot service that api/worker depend on with
#               service_completed_successfully, so N replicas starting at
#               once never race the schema (db.Migrate itself also takes a
#               Postgres advisory lock, belt and braces).
# api / worker: export ARENA_ROLE accordingly and exec the api binary,
#               which reads it (api: public HTTP + metrics, no sandbox
#               workers, no Docker needed; worker: proof workers + metrics,
#               no public HTTP listener).
# (no argument): exec the api binary as-is; ARENA_ROLE defaults to "all"
#               (single-process dev/small-deploy behaviour), matching this
#               script's previous behaviour except that migrations are no
#               longer run implicitly here — run `entrypoint.sh migrate`
#               (or `make migrate`) first.
case "${1:-}" in
  migrate)
    exec migrate
    ;;
  api|worker)
    export ARENA_ROLE="$1"
    exec api
    ;;
  *)
    exec api
    ;;
esac
