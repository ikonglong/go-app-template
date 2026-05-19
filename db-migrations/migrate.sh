#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------------------
# Load database config from environment variables / .env
# ---------------------------------------------------------------------------
load_db_config() {
    local env_file="$PROJECT_ROOT/.env"
    if [[ -f "$env_file" ]]; then
        local host="" port="" name="" user="" password=""

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

        : "${host:=${DATABASE__HOST:-localhost}}"
        : "${port:=${DATABASE__PORT:-5432}}"
        : "${name:=${DATABASE__NAME:-stg_medeo}}"
        : "${user:=${DATABASE__USER:-postgres}}"
        : "${password:=${DATABASE__PASSWORD:-}}"
    else
        local host="${DATABASE__HOST:-}"
        local port="${DATABASE__PORT:-5432}"
        local name="${DATABASE__NAME:-}"
        local user="${DATABASE__USER:-postgres}"
        local password="${DATABASE__PASSWORD:-}"

        if [[ -z "$host" || -z "$name" ]]; then
            echo "Error: .env not found at $env_file and DATABASE__HOST/DATABASE__NAME not set in environment" >&2
            exit 1
        fi
    fi

    export GOOSE_DRIVER="postgres"
    export GOOSE_DBSTRING="postgres://${user}:${password}@${host}:${port}/${name}"
}

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [args]

Commands:
  up                Migrate to the most recent version
  up-by-one         Migrate up by one version
  up-to VERSION     Migrate up to a specific version
  down              Roll back by one version
  down-to VERSION   Roll back to a specific version
  redo              Re-run the latest migration
  reset             Roll back all migrations
  status            Show migration status
  version           Print current DB version
  create NAME       Create a new SQL migration file
  validate          Check migration files without running them

Options:
  -allow-missing    Apply missing (out-of-order) migrations. Useful when
                    multiple branches add migrations concurrently and they
                    merge in a different order than their timestamps.
                    Can be combined with: up, up-by-one, up-to.

Examples:
  $(basename "$0") status
  $(basename "$0") up
  $(basename "$0") up -allow-missing
  $(basename "$0") down
  $(basename "$0") create "add_user_table"
EOF
    exit 1
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
if ! command -v goose &>/dev/null; then
    echo "Error: goose is not installed." >&2
    echo "Install: https://github.com/pressly/goose#install" >&2
    exit 1
fi

[[ $# -lt 1 ]] && usage

export GOOSE_MIGRATION_DIR="$SCRIPT_DIR/versions"

case "$1" in
    create)
        # Default to SQL migrations: `create NAME` → `create NAME sql`
        if [[ $# -eq 2 ]]; then
            set -- "$1" "$2" sql
        fi
        ;;
    validate)
        ;;
    *)
        load_db_config
        ;;
esac

cd "$PROJECT_ROOT"
exec goose "$@"
