#!/bin/sh
# Container start: apply migrations, make sure the first competition
# exists, then serve. Migrations and the seed are idempotent.
set -eu
migrate
seed -task city-day-planner
exec api
