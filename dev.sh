#!/usr/bin/env bash
# Development mode: Backend(:8787) + Flow agent(:8788) With frontend next dev(:5173) Run together..
# Frontend /api Inverse to Backend;Ctrl-C Quit Together.
#
# See single binary (front end embedded) mode README[Single binary]One section, no script..
set -euo pipefail
cd "$(dirname "$0")"

# End all subprocesses in this process group on exit (backend) + Frontend).
cleanup() { kill 0 2>/dev/null || true; }
trap cleanup EXIT INT TERM

# Backend (normal) go run,Do not embed frontend; co-op work agent On the count.[System Settings]Inner Configuration.
go run ./cmd/artex -addr :8787 -proxy 127.0.0.1:8788 &

# Frontend Thermal Update(Vite/Next dev server,/api Inversely. :8787).
( cd web && npm run dev ) &

echo "[dev] Backend :8787 / Agent :8788 / Frontend http://localhost:5173  (Ctrl-C Exit)"
wait
