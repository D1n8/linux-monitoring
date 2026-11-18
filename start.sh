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
if [[ -f PLATFORM ]]; then
  read -r bundle_platform < PLATFORM
  daemon_arch="$(docker info --format '{{.Architecture}}')"
  case "$daemon_arch" in
    amd64|x86_64) daemon_platform=linux/amd64 ;;
    arm64|aarch64) daemon_platform=linux/arm64 ;;
    *) daemon_platform="linux/$daemon_arch" ;;
  esac
  if [[ "$bundle_platform" != "$daemon_platform" ]]; then
    printf 'Нужен архив для %s; этот архив собран для %s.\n' "$daemon_platform" "$bundle_platform" >&2
    exit 1
  fi
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
if [[ "${1:-}" == --build ]]; then
  if [[ ! -f Dockerfile || ! -f compose.build.yaml ]]; then
    echo 'Этот архив содержит готовую поставку. Для сборки нужны исходники репозитория.' >&2
    exit 1
  fi
  docker compose -f compose.yaml -f compose.build.yaml build app
elif [[ -f goinmetrics-image.tar.gz ]]; then
  # Release archives pin the image in compose.yaml. Load it only when absent.
  image_ref="$(docker compose config --images app | head -n1)"
  if ! docker image inspect "$image_ref" >/dev/null 2>&1; then
    if [[ ! -f SHA256SUMS ]] || ! sha256sum --check --status SHA256SUMS; then
      echo 'Не совпала контрольная сумма файлов поставки. Загрузите архив заново.' >&2
      exit 1
    fi
    docker image load --input goinmetrics-image.tar.gz
    if ! docker image inspect "$image_ref" >/dev/null 2>&1; then
      echo 'Архив не содержит ожидаемый образ приложения.' >&2
      exit 1
    fi
  fi
fi
# Missing images are downloaded; compilation is possible only with --build.
if ! docker compose up -d --no-build --pull missing --wait --wait-timeout 180; then
  printf '\nЗапуск не выполнен. Проверьте доступ к готовым образам.\nДля разработки из исходников: ./start.sh --build\n' >&2
  exit 1
fi
printf '\nGoInMetrics: http://127.0.0.1:9099\n\nДля удалённого доступа на своём компьютере:\n  ssh -N -L 127.0.0.1:19099:127.0.0.1:9099 user@server\nЗатем откройте http://127.0.0.1:19099\n'
