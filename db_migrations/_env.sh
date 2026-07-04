#!/bin/bash
# Shared .env loading for db_migrations/ and codegen/ scripts.
# Source this file and call load_db_config. It populates:
#   DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD
# The caller is responsible for building its own connection strings
# (DSN, GOOSE_DBSTRING, etc.) from these variables.

load_db_config() {
    if [[ -z "${PROJECT_ROOT:-}" ]]; then
        echo "Error: PROJECT_ROOT must be set before calling load_db_config" >&2
        return 1
    fi

    local env_file="$PROJECT_ROOT/.env"
    local _host="" _port="" _name="" _user="" _password=""

    if [[ -f "$env_file" ]]; then
        while IFS='=' read -r key value; do
            [[ -z "$key" || "$key" == \#* ]] && continue
            key="$(echo "$key" | xargs)"
            value="$(echo "$value" | xargs)"

            case "$key" in
                DATABASE__HOST)     _host="$value" ;;
                DATABASE__PORT)     _port="$value" ;;
                DATABASE__NAME)     _name="$value" ;;
                DATABASE__USER)     _user="$value" ;;
                DATABASE__PASSWORD) _password="$value" ;;
            esac
        done < "$env_file"
    fi

    DB_HOST="${_host:-${DATABASE__HOST:-localhost}}"
    DB_PORT="${_port:-${DATABASE__PORT:-5432}}"
    DB_NAME="${_name:-${DATABASE__NAME:-}}"
    DB_USER="${_user:-${DATABASE__USER:-postgres}}"
    DB_PASSWORD="${_password:-${DATABASE__PASSWORD:-}}"

    if [[ -z "$DB_NAME" ]]; then
        echo "Error: DATABASE__NAME not set (.env or env)" >&2
        return 1
    fi
}
