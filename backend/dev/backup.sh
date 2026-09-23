#!/bin/sh
# Daily pg_dump, keep the last 7. Runs inside the backup container.
while true; do
  pg_dump -h postgres -U arena_migrate -d arena -Fc -f "/backups/arena-$(date +%F).dump" && echo "backup ok $(date)"
  ls -1t /backups/arena-*.dump 2>/dev/null | tail -n +8 | xargs -r rm --
  sleep 86400
done
