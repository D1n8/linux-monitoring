#!/usr/bin/env bash
# Package an already-built image. No credentials are read or written by this script.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
if (( $# != 3 )); then
  echo 'Usage: scripts/package-release.sh IMAGE linux/amd64|linux/arm64 OUTPUT_DIRECTORY' >&2
  exit 2
fi
image_ref="$1"
platform="$2"
output_dir="$3"
if [[ ! "$image_ref" =~ ^[a-z0-9][a-z0-9./:@_-]+$ ]] || [[ "$platform" != linux/amd64 && "$platform" != linux/arm64 ]]; then
  echo 'Invalid image reference or platform.' >&2
  exit 2
fi
actual_platform="$(docker image inspect "$image_ref" --format '{{.Os}}/{{.Architecture}}')"
if [[ "$actual_platform" != "$platform" ]]; then
  printf 'Image architecture mismatch: expected %s, got %s\n' "$platform" "$actual_platform" >&2
  exit 1
fi
mkdir -p -- "$output_dir"
output_dir="$(cd -- "$output_dir" && pwd)"
release_dir="$(mktemp -d)"
trap 'rm -rf -- "$release_dir"' EXIT
bundle="goinmetrics-${platform//\//-}"
mkdir "$release_dir/$bundle"
printf '%s\n' "$platform" > "$release_dir/$bundle/PLATFORM"
cp start.sh stop.sh "$release_dir/$bundle/"
# Compose supports an optional override but the distributed default pins this image.
sed "s|ghcr.io/d1n8/goinmetrics:latest|$image_ref|g" compose.yaml > "$release_dir/$bundle/compose.yaml"
docker image save "$image_ref" | gzip -n > "$release_dir/$bundle/goinmetrics-image.tar.gz"
cat > "$release_dir/$bundle/README.txt" <<EOF_NOTE
GoInMetrics · $platform

Requirements: Linux, Docker Engine 25+, Compose 2.20.2+.
Start: ./start.sh
Stop:  ./stop.sh
Open:  http://127.0.0.1:9099

The application image is included. PostgreSQL is downloaded as a ready image
on first start. No Go/Node.js installation or compilation is needed on the server.
Keep .env and Docker volumes to preserve the database password and history.
Never use "docker compose down -v" for a normal stop.
Image: $image_ref
EOF_NOTE
(cd "$release_dir/$bundle" && sha256sum compose.yaml start.sh stop.sh goinmetrics-image.tar.gz README.txt PLATFORM > SHA256SUMS)
# Stable file ordering and timestamps make packages reproducible for the same image.
tar --sort=name --mtime='UTC 2026-01-01' --owner=0 --group=0 --numeric-owner -czf "$output_dir/$bundle.tar.gz" -C "$release_dir" "$bundle"
(cd "$output_dir" && sha256sum "$bundle.tar.gz" > "$bundle.tar.gz.sha256")
printf '%s\n' "$output_dir/$bundle.tar.gz"
