#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
if (( $# > 1 )) || [[ "${1:-}" != "" && "${1:-}" != --build ]]; then
  echo 'Использование: ./start.sh [--build]' >&2
  exit 2
fi
if [[ "$(uname -s)" != Linux ]]; then
  echo 'GoInMetrics MVP поддерживает Linux с Docker Engine 25+ и Compose 2.20.2+.' >&2
  exit 1
fi
if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  echo 'Установите Docker Engine 25+ с Compose 2.20.2+: https://docs.docker.com/engine/install/' >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo 'Нет доступа к Docker Engine. Запустите Docker и проверьте права текущего пользователя.' >&2
  exit 1
fi
# Atomic creation, no overwrites: the password must survive subsequent starts.
if [[ ! -f .env ]]; then
  umask 077
  password="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  (set -o noclobber; printf 'POSTGRES_PASSWORD=%s\n' "$password" > .env) || true
fi
if ! grep -Eq '^POSTGRES_PASSWORD=[a-f0-9]{64}$' .env; then
  echo 'Некорректный .env: ожидается POSTGRES_PASSWORD из 64 hex-символов. Не удаляйте пароль существующей БД.' >&2
  exit 1
fi
if [[ "${1:-}" == --build ]] || ! docker image inspect goinmetrics-app:latest >/dev/null 2>&1; then
  docker compose up -d --build --wait --wait-timeout 180
else
  printf 'Используется локальный образ. Для пересборки: ./start.sh --build\n'
  docker compose up -d --no-build --pull missing --wait --wait-timeout 180
fi
printf '\nGoInMetrics: http://127.0.0.1:9099\n\nДля удалённого доступа на своём компьютере:\n  ssh -N -L 127.0.0.1:19099:127.0.0.1:9099 user@server\nЗатем откройте http://127.0.0.1:19099\n'
