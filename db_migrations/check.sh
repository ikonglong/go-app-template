#!/usr/bin/env bash
# Validate goose SQL migration files.
# Checks: duplicate timestamps, goose validate (syntax & directives).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSIONS_DIR="$SCRIPT_DIR/versions"

if [[ ! -d "$VERSIONS_DIR" ]]; then
    echo "ERROR: Directory $VERSIONS_DIR not found" >&2
    exit 1
fi

# ---- Check 1: duplicate timestamps ----
dupes=$(ls "$VERSIONS_DIR"/*.sql 2>/dev/null \
    | xargs -n1 basename \
    | sed -E 's/^([0-9]{14}).*/\1/' \
    | sort | uniq -d)

if [[ -n "$dupes" ]]; then
    echo "ERROR: Duplicate timestamps detected:"
    for ts in $dupes; do
        echo "  $ts:"
        ls "$VERSIONS_DIR"/${ts}*.sql | xargs -n1 basename | sed 's/^/    /'
    done
    exit 1
fi

# ---- Check 2: goose validate (syntax & directives) ----
exec env GOOSE_MIGRATION_DIR="$VERSIONS_DIR" goose validate
