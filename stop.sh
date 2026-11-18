#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
docker compose stop
printf '\nGoInMetrics и PostgreSQL остановлены. Данные сохранены. Повторный запуск: ./start.sh\n'
