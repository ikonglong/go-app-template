#!/bin/bash
# ==============================================================================
# Director V2 - Production-ready automated migration runner (Goose)
# Use in: Docker, Kubernetes, CI/CD
# Features: Non-interactive, intelligent retry, comprehensive diagnostics
# ==============================================================================

set -eo pipefail

readonly EXIT_SUCCESS=0
readonly EXIT_DB_CONNECTION_FAILED=2
readonly EXIT_MIGRATION_FAILED=3

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load .env if exists (local dev / CI; in Docker env vars are already set)
if [ -f "$PROJECT_ROOT/.env" ]; then
    set -a
    while IFS= read -r line || [ -n "$line" ]; do
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        [[ -z "$line" ]] && continue
        export "$line" 2>/dev/null || true
    done < "$PROJECT_ROOT/.env"
    set +a
fi

# ==============================================================================
# Helpers
# ==============================================================================

log_info()    { echo "ℹ️  $1"; }
log_success() { echo "✅ $1"; }
log_error()   { echo "❌ $1" >&2; }
log_warning() { echo "⚠️  $1"; }
print_separator() { echo "===================================="; }

# ==============================================================================
# Configuration
# ==============================================================================

DB_HOST="${DATABASE__HOST:-localhost}"
DB_PORT="${DATABASE__PORT:-5432}"
DB_USER="${DATABASE__USER:-postgres}"
DB_PASSWORD="${DATABASE__PASSWORD}"
DB_NAME="${DATABASE__NAME:-stg_medeo}"
MIGRATION_ENABLED="${MIGRATION__ENABLED:-true}"
MIGRATION_STRICT="${MIGRATION__STRICT:-false}"
MAX_WAIT_ATTEMPTS="${DATABASE__WAIT_ATTEMPTS:-30}"
WAIT_INTERVAL="${DATABASE__WAIT_INTERVAL:-2}"

export GOOSE_DRIVER="postgres"
export GOOSE_DBSTRING="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}"
export GOOSE_MIGRATION_DIR="$SCRIPT_DIR/versions"

# ==============================================================================
# Database Connection Check
# ==============================================================================

wait_for_database() {
    log_info "Waiting for database at ${DB_HOST}:${DB_PORT}..."
    log_info "Database: ${DB_NAME}, User: ${DB_USER}"

    local attempt=1

    while [ $attempt -le $MAX_WAIT_ATTEMPTS ]; do
        if PGPASSWORD="${DB_PASSWORD}" pg_isready \
            -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
            -t 3 &>/dev/null; then
            log_success "Database is ready!"
            return $EXIT_SUCCESS
        fi

        if [ $((attempt % 5)) -eq 1 ] && [ $attempt -gt 1 ]; then
            echo "   Attempt $attempt/$MAX_WAIT_ATTEMPTS - Still waiting..."
        else
            echo "   Attempt $attempt/$MAX_WAIT_ATTEMPTS - Database not ready, waiting..."
        fi

        sleep $WAIT_INTERVAL
        attempt=$((attempt + 1))
    done

    local total_wait=$((MAX_WAIT_ATTEMPTS * WAIT_INTERVAL))

    log_error "Database connection timeout after $MAX_WAIT_ATTEMPTS attempts (${total_wait}s)"
    log_error ""
    log_error "Troubleshooting checklist:"
    log_error "  1. Database server running: pg_isready -h ${DB_HOST} -p ${DB_PORT}"
    log_error "  2. Host reachable: ping ${DB_HOST}"
    log_error "  3. Port accessible: telnet ${DB_HOST} ${DB_PORT}"
    log_error "  4. Database exists: psql -h ${DB_HOST} -p ${DB_PORT} -U ${DB_USER} -l"
    log_error "  5. Check credentials for user '${DB_USER}'"
    log_error ""
    log_error "Current configuration:"
    log_error "  DATABASE__HOST=${DB_HOST}"
    log_error "  DATABASE__PORT=${DB_PORT}"
    log_error "  DATABASE__NAME=${DB_NAME}"
    log_error "  DATABASE__USER=${DB_USER}"

    return $EXIT_DB_CONNECTION_FAILED
}

# ==============================================================================
# Migration Execution
# ==============================================================================

run_goose_migrations() {
    log_info "Running Goose migrations..."

    if goose up -allow-missing; then
        log_success "Migrations completed successfully"
        return $EXIT_SUCCESS
    else
        local exit_code=$?
        log_error "Goose migration failed with exit code: $exit_code"
        return $EXIT_MIGRATION_FAILED
    fi
}

# ==============================================================================
# Main
# ==============================================================================

main() {
    print_separator
    echo "Director V2 - Database Migration (Goose)"
    print_separator
    echo ""

    log_info "Migration Configuration:"
    echo "   Environment: ${APP__ENV:-staging}"
    echo "   Database Host: ${DB_HOST}"
    echo "   Database Port: ${DB_PORT}"
    echo "   Database Name: ${DB_NAME}"
    echo "   Database User: ${DB_USER}"
    echo "   Migration Enabled: ${MIGRATION_ENABLED}"
    echo "   Migration Strict: ${MIGRATION_STRICT}"
    echo ""

    if [ "${MIGRATION_ENABLED}" = "false" ]; then
        log_warning "Migrations are disabled (MIGRATION__ENABLED=false)"
        return $EXIT_SUCCESS
    fi

    if ! wait_for_database; then
        log_error "Cannot connect to database"
        if [ "${MIGRATION_STRICT}" = "true" ]; then
            log_error "MIGRATION__STRICT=true, exiting with code $EXIT_DB_CONNECTION_FAILED"
            return $EXIT_DB_CONNECTION_FAILED
        else
            log_warning "MIGRATION__STRICT=false, continuing without migrations"
            return $EXIT_SUCCESS
        fi
    fi

    echo ""

    if ! run_goose_migrations; then
        if [ "${MIGRATION_STRICT}" = "true" ]; then
            log_error "MIGRATION__STRICT=true, exiting with code $EXIT_MIGRATION_FAILED"
            return $EXIT_MIGRATION_FAILED
        else
            log_warning "MIGRATION__STRICT=false, continuing despite migration failure"
            return $EXIT_SUCCESS
        fi
    fi

    echo ""
    print_separator
    log_success "Migration process completed"
    print_separator

    return $EXIT_SUCCESS
}

main
exit $?
