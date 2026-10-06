#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

git pull --ff-only
mode="${1:-docker}"
case "$mode" in
  docker)
    command -v docker >/dev/null || { echo 'Docker is required.' >&2; exit 1; }
    docker compose up -d --build
    echo 'Updated application: http://localhost:8787'
    ;;
  local)
    command -v go >/dev/null || { echo 'Go 1.26 or newer is required.' >&2; exit 1; }
    command -v npm >/dev/null || { echo 'Node.js 24 or newer is required.' >&2; exit 1; }
    ARTEX_OUTPUT=artex ARTEX_COMPRESS=0 ./build.sh
    echo 'Restart the local ARTEX process to use the new build.'
    ;;
  *)
    echo 'Usage: ./update.sh [docker|local]' >&2
    exit 2
    ;;
esac
