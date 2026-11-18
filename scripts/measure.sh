#!/usr/bin/env bash
# Collect observations; no synthetic performance guarantees.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
seconds="${1:-60}"
if [[ ! "$seconds" =~ ^[0-9]+$ ]] || (( seconds < 1 || seconds > 3600 )); then
  echo 'Usage: ./scripts/measure.sh [seconds: 1..3600]' >&2
  exit 1
fi
printf '# GoInMetrics measurements (UTC)\n'
date -u --iso-8601=seconds
uname -srmo
printf '\n# CPU count / memory\n'
getconf _NPROCESSORS_ONLN
awk '/MemTotal|MemAvailable/ {print}' /proc/meminfo
printf '\n# Container observations (CPU is sampled, not a baseline comparison)\n'
for ((i=0; i<seconds; i+=5)); do
  date -u +%FT%TZ
  docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} memory={{.MemUsage}} io={{.BlockIO}}' "$(docker compose ps -q app)" "$(docker compose ps -q postgres)"
  if ((i+5<seconds)); then sleep 5; fi
done
printf '\n# HTTP status and response time (seconds)\n'
curl --silent --output /dev/null --write-out 'snapshot: %{http_code} %{time_total}\n' http://127.0.0.1:9099/api/snapshot
curl --silent --output /dev/null --write-out 'history: %{http_code} %{time_total}\n' http://127.0.0.1:9099/api/history
printf '\n# Observer health\n'
curl --silent --show-error http://127.0.0.1:9099/api/health
printf '\n# Actual database size\n'
docker compose exec -T postgres sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT pg_size_pretty(pg_database_size(current_database()))"'
