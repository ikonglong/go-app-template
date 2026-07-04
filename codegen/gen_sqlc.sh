#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0")

Regenerate sqlc code from SQL files on disk (no database connection needed).
Reads schema from db_migrations/versions/ and queries from
internal/adapter/repo/sqlc/query/ (or repo/query/ in a generated project).
EOF
}

for arg in "$@"; do
    case "$arg" in
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown flag: $arg" >&2; usage; exit 1 ;;
    esac
done

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
if ! command -v sqlc &>/dev/null; then
    echo "Error: sqlc is not installed." >&2
    echo "Install: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest" >&2
    exit 1
fi

cd "$PROJECT_ROOT"
exec sqlc generate -f codegen/sqlc.yaml
