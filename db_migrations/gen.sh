#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------------------
# Load database config from .env / environment (mirrors migrate.sh)
# ---------------------------------------------------------------------------
load_db_config() {
    local env_file="$PROJECT_ROOT/.env"
    local host="" port="" name="" user="" password=""

    if [[ -f "$env_file" ]]; then
        while IFS='=' read -r key value; do
            [[ -z "$key" || "$key" == \#* ]] && continue
            key="$(echo "$key" | xargs)"
            value="$(echo "$value" | xargs)"

            case "$key" in
                DATABASE__HOST)     host="$value" ;;
                DATABASE__PORT)     port="$value" ;;
                DATABASE__NAME)     name="$value" ;;
                DATABASE__USER)     user="$value" ;;
                DATABASE__PASSWORD) password="$value" ;;
            esac
        done < "$env_file"
    fi

    : "${host:=${DATABASE__HOST:-localhost}}"
    : "${port:=${DATABASE__PORT:-5432}}"
    : "${name:=${DATABASE__NAME:-}}"
    : "${user:=${DATABASE__USER:-postgres}}"
    : "${password:=${DATABASE__PASSWORD:-}}"

    if [[ -z "$name" ]]; then
        echo "Error: DATABASE__NAME not set (.env or env)" >&2
        exit 1
    fi

    export DSN="postgresql://${user}:${password}@${host}:${port}/${name}?sslmode=disable"
    export DB_LABEL="${name}@${host}:${port}"
}

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") [JET_FLAGS...]

Regenerate go-jet code from the live database schema. Reads connection
config from .env using the same DATABASE__* keys as migrate.sh.

Any extra flags are appended to the jet invocation; later values of the
same flag win, so you can override any default below.

Defaults injected:
  -source=postgres
  -dsn=<built from .env>
  -schema=public
  -path=./internal/adapter/repo/jet/gen
  -rel-model-path=./record
  -ignore-tables=goose_db_version

Examples:
  $(basename "$0")
  $(basename "$0") -tables=account
  $(basename "$0") -schema=auth
  $(basename "$0") -path=./gen-throwaway
EOF
}

for arg in "$@"; do
    case "$arg" in
        -h|--help) usage; exit 0 ;;
    esac
done

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
if ! command -v jet &>/dev/null; then
    echo "Error: jet is not installed." >&2
    echo "Install: go install github.com/go-jet/jet/v2/cmd/jet@latest" >&2
    exit 1
fi

load_db_config

cd "$PROJECT_ROOT"

# jet rejects -tables and -ignore-tables simultaneously, so only inject the
# ignore default when the caller is not using an explicit allow-list.
user_set_tables=false
for arg in "$@"; do
    case "$arg" in
        -tables|-tables=*) user_set_tables=true ;;
    esac
done

defaults=(
    -source=postgres
    -dsn="$DSN"
    -schema=public
    -path=./internal/adapter/repo/jet/gen
    -rel-model-path=./record
)
$user_set_tables || defaults+=(-ignore-tables=goose_db_version)

echo "Generating jet code: ${DB_LABEL} -> ./internal/adapter/repo/jet/gen"
exec jet "${defaults[@]}" "$@"
