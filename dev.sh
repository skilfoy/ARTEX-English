#!/usr/bin/env bash
# Development mode: backend (:8787) + traffic proxy (:8788) and the frontend
# next dev server (:3000) run together.
# The frontend proxies /api to the backend; Ctrl-C stops both.
#
# The embedded-frontend binary is described under "Build the English application"
# in the README and does not use this script.
set -euo pipefail
cd "$(dirname "$0")"

# On exit, stop every child in this process group (backend + frontend).
cleanup() { kill 0 2>/dev/null || true; }
trap cleanup EXIT INT TERM

# Backend via a normal go run, without the embedded frontend. The number of
# concurrent work agents is set under System Settings.
go run ./cmd/artex -addr :8787 -proxy 127.0.0.1:8788 &

# Frontend hot reload (Next.js dev server; /api is proxied to :8787).
( cd web && npm run dev ) &

echo "[dev] backend :8787 / proxy :8788 / frontend http://localhost:3000  (Ctrl-C to exit)"
wait
