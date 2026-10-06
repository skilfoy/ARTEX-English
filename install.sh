#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

mode="${1:-docker}"
case "$mode" in
  docker)
    command -v docker >/dev/null || { echo 'Install Docker and Docker Compose first.' >&2; exit 1; }
    docker compose version >/dev/null
    if [ ! -f .env ]; then
      cp .env.example .env
      password="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
      sed -i.bak "s/replace-with-a-long-random-password/$password/" .env
      rm -f .env.bak
      chmod 0600 .env
      echo 'Created .env with a random PostgreSQL password.'
    fi
    docker compose up -d --build
    echo 'Application: http://localhost:8787'
    ;;
  local)
    command -v go >/dev/null || { echo 'Install Go 1.26 or newer.' >&2; exit 1; }
    command -v npm >/dev/null || { echo 'Install Node.js 24 or newer.' >&2; exit 1; }
    if [ -z "${ARTEX_PG_DSN:-}" ] && [ ! -f config.json ]; then
      cp config.example.json config.json
      echo 'Configure PostgreSQL in config.json or ARTEX_PG_DSN, then run this command again.' >&2
      exit 1
    fi
    ARTEX_OUTPUT=artex ARTEX_COMPRESS=0 ./build.sh
    ./start.sh
    ;;
  *)
    echo 'Usage: ./install.sh [docker|local]' >&2
    exit 2
    ;;
esac
