#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------------------
# Load database config from shared helper
# ---------------------------------------------------------------------------
source "$SCRIPT_DIR/_env.sh"

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
        export GOOSE_DRIVER="postgres"
        export GOOSE_DBSTRING="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}"
        ;;
esac

cd "$PROJECT_ROOT"
exec goose "$@"
