#!/usr/bin/env bash
# Backup Hodhod Postgres (run on host with docker compose).
set -euo pipefail
OUT="hodhod-backup-$(date +%Y%m%d-%H%M%S).sql"
docker compose exec -T hodhod-db pg_dump -U hodhod hodhod > "$OUT"
echo "Wrote $OUT"
