#!/bin/sh
# Start ARTEX and handle restart requests from the built-in updater.
# Usage: ./start.sh [-addr :9000]
set -u

cd "$(dirname "$0")" || exit 1
BIN=./artex
[ -x "$BIN" ] || { echo "[artex] Executable not found: $BIN" >&2; exit 1; }

RESTART_CODE=75
MAX_DELAY=60
child=0
stopping=0

# Forward shutdown signals to the Go process for graceful termination.
forward() {
    stopping=1
    if [ "$child" -ne 0 ]; then
        kill -TERM "$child" 2>/dev/null || true
    fi
}
trap forward INT TERM

delay=1
while :; do
    "$BIN" "$@" &
    child=$!
    wait "$child"
    code=$?
    if [ "$code" -gt 128 ]; then
        wait "$child"
        code=$?
    fi
    child=0

    if [ "$stopping" -eq 1 ]; then
        echo "[artex] Stopped"
        exit 0
    fi

    case "$code" in
        0)
            echo "[artex] Exited normally"
            exit 0
            ;;
        "$RESTART_CODE")
            echo "[artex] Restart requested"
            delay=1
            ;;
        *)
            echo "[artex] Exit code $code; restarting in ${delay}s" >&2
            sleep "$delay"
            delay=$((delay * 2))
            [ "$delay" -gt "$MAX_DELAY" ] && delay=$MAX_DELAY
            ;;
    esac
done
