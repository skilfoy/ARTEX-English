#!/usr/bin/env bash
# =============================================================================
# ARTEX Manager password reset script
#
# Login username fixed to ARTEX;Password by bcrypt Hash is in the database. settings Table
# auth.password_hash key. When this script is connected to the database, use pgcrypto Generate in library bcrypt Hash.
# and write back the key——Verify with Backend Login(golang.org/x/crypto/bcrypt)Full compatibility.
#
# Two deployments:
#   local (Default)—— Live host direct use psql Connect database. Connect information according to the following priority:
#                    Command Line Parameters > --dsn/$ARTEX_PG_DSN > config.json of database.*
#   docker        —— Pass `docker compose exec`(or `docker exec`)at postgres
#                    Execute inside the container psql(compose Default not exposed to host 5432,So go inside the container.).
#
# Example usage:
#   ./reset-password.sh                          # Local, autoread config.json/Environment, interactive input of new passwords
#   ./reset-password.sh -p 'NewPass!'            # Local, give the new password directly.
#   ./reset-password.sh --dsn postgres://u:p@h:5432/artex
#   ./reset-password.sh -H 127.0.0.1 -P 5433 -U autopentest -W pass -d artex
#   ./reset-password.sh -m docker                # docker Deployment (read .env of POSTGRES_*)
#   ./reset-password.sh -m docker -c pgContainer Name --exec docker
#
# Security: new password passed by environment variable + psql \getenv Import (not in process) argv),Use both :'var'
# Automatic conversion (prevention) SQL Injection; database password Yes. PGPASSWORD Pass it. Same thing. argv.
# =============================================================================
set -euo pipefail

PASS_KEY="auth.password_hash"
BCRYPT_COST=10

MODE=""            # local | docker(Empty=Automatic determination)
DSN=""
HOST="" PORT="" USER="" DBPASS="" DBNAME="" SSLMODE=""
CONFIG=""
CONTAINER=""       # docker Mode postgres Service/Container name (default) postgres)
EXEC_KIND=""       # compose | docker(docker What's the pattern? exec;Empty=Automatic)
NEWPASS=""
ASSUME_YES=0

die() { echo "Error:$*" >&2; exit 1; }
info() { echo "· $*" >&2; }

usage() { sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'; exit 0; }

# ---- Parameter Parsing -------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    -m|--mode)        MODE="${2:-}"; shift 2 ;;
    --dsn)            DSN="${2:-}"; shift 2 ;;
    -H|--host)        HOST="${2:-}"; shift 2 ;;
    -P|--port)        PORT="${2:-}"; shift 2 ;;
    -U|--user)        USER="${2:-}"; shift 2 ;;
    -W|--db-password) DBPASS="${2:-}"; shift 2 ;;
    -d|--dbname)      DBNAME="${2:-}"; shift 2 ;;
    --sslmode)        SSLMODE="${2:-}"; shift 2 ;;
    --config)         CONFIG="${2:-}"; shift 2 ;;
    -c|--container)   CONTAINER="${2:-}"; shift 2 ;;
    --exec)           EXEC_KIND="${2:-}"; shift 2 ;;
    -p|--new-password) NEWPASS="${2:-}"; shift 2 ;;
    -y|--yes)         ASSUME_YES=1; shift ;;
    -h|--help)        usage ;;
    *) die "Unknown parameter:$1(-h View Usage)" ;;
  esac
done

# ---- from config.json Read database.*(Only local Mode, without visible connection)-----
# Priority python3 (b) Analysis (hardness); python3 Back then. grep(config.json Sets the field).
read_config_json() {
  local path="$1"
  [[ -f "$path" ]] || return 1
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$path" <<'PY'
import json, sys
try:
    d = json.load(open(sys.argv[1])).get("database", {})
except Exception:
    sys.exit(1)
# Support directly to dsn,or by field
if d.get("dsn"):
    print("DSN\t" + d["dsn"]); sys.exit(0)
for k in ("host","port","user","password","dbname","sslmode"):
    if d.get(k) is not None:
        print(k.upper() + "\t" + str(d[k]))
PY
  else
    # Very simple backup: keys by key grep(Value is string or number)
    local k
    for k in host port user password dbname sslmode; do
      local v
      v=$(grep -oE "\"$k\"[[:space:]]*:[[:space:]]*(\"[^\"]*\"|[0-9]+)" "$path" 2>/dev/null \
            | head -1 | sed -E "s/.*:[[:space:]]*//; s/^\"//; s/\"$//") || true
      [[ -n "$v" ]] && echo -e "${k^^}\t$v"
    done
  fi
}

apply_config_fields() {
  local line key val
  while IFS=$'\t' read -r key val; do
    [[ -z "$key" ]] && continue
    case "$key" in
      DSN)      [[ -z "$DSN" ]] && DSN="$val" ;;
      HOST)     [[ -z "$HOST" ]] && HOST="$val" ;;
      PORT)     [[ -z "$PORT" ]] && PORT="$val" ;;
      USER)     [[ -z "$USER" ]] && USER="$val" ;;
      PASSWORD) [[ -z "$DBPASS" ]] && DBPASS="$val" ;;
      DBNAME)   [[ -z "$DBNAME" ]] && DBNAME="$val" ;;
      SSLMODE)  [[ -z "$SSLMODE" ]] && SSLMODE="$val" ;;
    esac
  done
}

# ---- Automatic award mode ---------------------------------------------------------
if [[ -z "$MODE" ]]; then
  if [[ -n "$DSN$HOST$USER$DBNAME" || -n "${ARTEX_PG_DSN:-}" || -f "${CONFIG:-config.json}" ]]; then
    MODE="local"
  elif command -v docker >/dev/null 2>&1 && [[ -f docker-compose.yml ]]; then
    MODE="docker"
  else
    MODE="local"
  fi
fi
info "Deployment pattern:$MODE"

# ---- Collect new password -----------------------------------------------------------
if [[ -z "$NEWPASS" ]]; then
  read -r -s -p "Enter a new password (username fixed to ARTEX):" NEWPASS; echo >&2
  [[ -n "$NEWPASS" ]] || die "Password cannot be empty"
  read -r -s -p "Enter again to confirm:" NEWPASS2; echo >&2
  [[ "$NEWPASS" == "$NEWPASS2" ]] || die "Double input is inconsistent"
fi
[[ -n "$NEWPASS" ]] || die "Password cannot be empty"

# Give the password to the environment variable. psql(\getenv Read, do not enter argv/ps)
export ARTEX_RESET_NEWPASS="$NEWPASS"

# Library Generation bcrypt and upsert;Password :'newpw' Autotransformation.CREATE EXTENSION Wait.,
# Errors are reported here if the database character has no extension (see below for hint) run Other Organiser).
SQL=$(cat <<SQL
\\set ON_ERROR_STOP on
\\getenv newpw ARTEX_RESET_NEWPASS
CREATE EXTENSION IF NOT EXISTS pgcrypto;
INSERT INTO settings(key, value)
VALUES ('$PASS_KEY', crypt(:'newpw', gen_salt('bf', $BCRYPT_COST)))
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();
SQL
)

# ---- Execute -----------------------------------------------------------------
if [[ "$MODE" == "local" ]]; then
  # Connect information priority: command line > --dsn/$ARTEX_PG_DSN > config.json
  if [[ -z "$DSN" && -z "$HOST$USER$DBNAME" ]]; then
    [[ -n "${ARTEX_PG_DSN:-}" ]] && DSN="$ARTEX_PG_DSN"
  fi
  if [[ -z "$DSN" && -z "$HOST$USER$DBNAME" ]]; then
    cfg="${CONFIG:-config.json}"
    if [[ -f "$cfg" ]]; then
      info "from $cfg Read Database Configuration"
      apply_config_fields < <(read_config_json "$cfg")
    fi
  fi

  command -v psql >/dev/null 2>&1 || die "Not found psql(Please install postgresql-client,Or change it. -m docker)"

  declare -a PSQL_ARGS=()
  if [[ -n "$DSN" ]]; then
    PSQL_ARGS=("$DSN")
    target="$DSN"
  else
    [[ -n "$USER"   ]] || die "Missing database users(-U)Or effective. config.json/DSN"
    [[ -n "$DBNAME" ]] || die "Missing database name(-d)Or effective. config.json/DSN"
    HOST="${HOST:-127.0.0.1}"; PORT="${PORT:-5432}"; SSLMODE="${SSLMODE:-disable}"
    PSQL_ARGS=(-h "$HOST" -p "$PORT" -U "$USER" -d "$DBNAME")
    [[ -n "$SSLMODE" ]] && export PGSSLMODE="$SSLMODE"
    [[ -n "$DBPASS" ]] && export PGPASSWORD="$DBPASS"
    target="$USER@$HOST:$PORT/$DBNAME"
  fi

  info "Target database:$target"
  if [[ "$ASSUME_YES" -ne 1 ]]; then
    read -r -p "Confirm to Reset in the Library ARTEX Password?[y/N] " ans
    [[ "$ans" == "y" || "$ans" == "Y" ]] || die "Cancelled"
  fi

  if ! printf '%s\n' "$SQL" | psql "${PSQL_ARGS[@]}" -v ON_ERROR_STOP=1 -q >/dev/null; then
    die "Writing failed. If you report pgcrypto Permissions/Missing, use a role with build-up permission, or start manually CREATE EXTENSION pgcrypto."
  fi

else
  # ---- docker ----
  command -v docker >/dev/null 2>&1 || die "Not found docker"
  CONTAINER="${CONTAINER:-postgres}"

  # Choose exec Modalities: priority docker compose exec(Service name) docker exec(Container Name)
  if [[ -z "$EXEC_KIND" ]]; then
    if docker compose version >/dev/null 2>&1 && [[ -f docker-compose.yml ]]; then
      EXEC_KIND="compose"
    else
      EXEC_KIND="docker"
    fi
  fi

  # Inside the container psql Documentary: Priority command line, second .env of POSTGRES_*,Back up. compose Default(artex)
  if [[ -f .env ]]; then
    # shellcheck disable=SC1091
    set -a; . ./.env; set +a
  fi
  DUSER="${USER:-${POSTGRES_USER:-artex}}"
  DNAME="${DBNAME:-${POSTGRES_DB:-artex}}"
  [[ -n "$DBPASS" ]] && export PGPASSWORD="$DBPASS"
  [[ -z "${PGPASSWORD:-}" && -n "${POSTGRES_PASSWORD:-}" ]] && export PGPASSWORD="$POSTGRES_PASSWORD"

  info "Target: Containers $CONTAINER Within psql -U $DUSER -d $DNAME(exec=$EXEC_KIND)"
  if [[ "$ASSUME_YES" -ne 1 ]]; then
    read -r -p "Confirm to reset the container database ARTEX Password?[y/N] " ans
    [[ "$ans" == "y" || "$ans" == "Y" ]] || die "Cancelled"
  fi

  # -e Only a name without value. → From the current environment, the password does not appear docker Command argv inside.
  declare -a EXEC_CMD
  if [[ "$EXEC_KIND" == "compose" ]]; then
    EXEC_CMD=(docker compose exec -T -e ARTEX_RESET_NEWPASS -e PGPASSWORD "$CONTAINER"
              psql -U "$DUSER" -d "$DNAME" -v ON_ERROR_STOP=1 -q)
  else
    EXEC_CMD=(docker exec -i -e ARTEX_RESET_NEWPASS -e PGPASSWORD "$CONTAINER"
              psql -U "$DUSER" -d "$DNAME" -v ON_ERROR_STOP=1 -q)
  fi

  if ! printf '%s\n' "$SQL" | "${EXEC_CMD[@]}" >/dev/null; then
    die "Writing failed. Please confirm the name of the container.(-c),Database Account(.env of POSTGRES_*),And the character. pgcrypto Permissions."
  fi
fi

unset ARTEX_RESET_NEWPASS
echo "✓ Resetd ARTEX The administrator password. Use username ARTEX + New password login (service not to restart))."
