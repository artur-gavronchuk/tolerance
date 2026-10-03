#!/bin/sh
set -eu

# Usage: entrypoint.sh [migrate]
#
# migrate:      run cmd/migrate once and exit (compose's one-shot service).
# (no argument): run the api — HTTP plus every worker, one process.
case "${1:-}" in
  migrate)
    exec migrate
    ;;
  *)
    exec api
    ;;
esac
