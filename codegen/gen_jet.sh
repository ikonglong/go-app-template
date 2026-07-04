#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------------------
# Load database config from shared helper
# ---------------------------------------------------------------------------
source "$PROJECT_ROOT/db_migrations/_env.sh"

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
DSN="postgresql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
DB_LABEL="${DB_NAME}@${DB_HOST}:${DB_PORT}"

cd "$PROJECT_ROOT"

# jet rejects -tables and -ignore-tables simultaneously, so only inject the
# ignore default when the caller is not using an explicit allow-list.
# Also extract overridden schema for the post-generation flatten step.
user_set_tables=false
schema="public"
for arg in "$@"; do
    case "$arg" in
        -tables|-tables=*) user_set_tables=true ;;
        -schema=*) schema="${arg#-schema=}" ;;
    esac
done

defaults=(
    -source=postgres
    -dsn="$DSN"
    -schema="$schema"
    -path=./internal/adapter/repo/jet/gen
    -rel-model-path=./record
)
$user_set_tables || defaults+=(-ignore-tables=goose_db_version)

gen_dir="./internal/adapter/repo/jet/gen"
# DB_LABEL is "name@host:port" — extract the db name for path construction.
db_name="${DB_LABEL%%@*}"
jet_out_dir="$gen_dir/$db_name/$schema"

echo "Generating jet code: ${DB_LABEL} -> $gen_dir"
jet "${defaults[@]}" "$@"

# Jet always nests output under <dbname>/<schema>/. Flatten it so generated
# code sits directly at gen/record/ and gen/table/.
rm -rf "$gen_dir/record" "$gen_dir/table"
mv "$jet_out_dir/record" "$gen_dir/record"
mv "$jet_out_dir/table" "$gen_dir/table"
rm -rf "$gen_dir/$db_name"
echo "Flattened: $gen_dir/record $gen_dir/table"
