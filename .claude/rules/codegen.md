---
paths:
  - "db_migrations/**"
---

# Code Generation & DB Migrations

> Scope: the **tooling** — regenerating jet code (`codegen/gen_jet.sh`) and authoring/applying/validating migrations (`migrate.sh`, `upgrade.sh`, `check.sh`). For how to write **safe migration SQL** (idempotent, non-blocking), see `.claude/db_migrations.md`.

## Migrations: `migrate.sh`

Wraps [goose](https://github.com/pressly/goose). Reads connection from `.env` (`DATABASE__*` keys) and forwards remaining args to `goose`.

```bash
./db_migrations/migrate.sh status
./db_migrations/migrate.sh up
./db_migrations/migrate.sh create add_orders_table   # writes to versions/
```

Migration files: `versions/<timestamp>_<name>.sql` — goose-format with `-- +goose Up` / `-- +goose Down` sections.

`migrate.sh` is the **local-dev** wrapper. Two siblings cover the rest of the lifecycle:

- **`upgrade.sh`** — the **non-interactive runner for Docker / Kubernetes / CI/CD**. Reads the same `.env` (`DATABASE__*`) connection and applies pending goose migrations, with retry and diagnostic output, exiting non-zero on a connection or migration failure. This is the production apply path; `migrate.sh up` is its interactive dev counterpart.
- **`check.sh`** — validates the files under `versions/` before they are applied: it flags duplicate timestamps and runs `goose validate` (syntax + directive checks). Run it after authoring a migration and in CI.

## Code Generation: `gen_jet.sh`

Wraps the [go-jet](https://github.com/go-jet/jet) generator. Reads `.env` for DB connection; everything after the script name passes through to `jet`.

```bash
./codegen/gen_jet.sh                  # default: full schema, skip goose_db_version
./codegen/gen_jet.sh -tables=account  # allow-list mode
./codegen/gen_jet.sh -schema=auth     # any jet flag pass-through; later values win
./codegen/gen_jet.sh -h               # show injected defaults
```

### Defaults injected

```
-source=postgres
-dsn=<built from .env>
-schema=public
-path=./internal/adapter/repo/jet/gen
-rel-model-path=./record           ← package name becomes "record" not "model"
-ignore-tables=goose_db_version    ← suppresses the migration tracking table
```

### Caveat: `-tables` and `-ignore-tables` are mutually exclusive

`jet` rejects them together with an explicit error. `gen_jet.sh` detects user-supplied `-tables` and drops the `-ignore-tables` default automatically. **Don't pass both manually.**

## Code Generation: `gen_sqlc.sh`

Wraps [sqlc](https://github.com/sqlc-dev/sqlc). No database connection needed — sqlc reads SQL schema and query files from disk.

```bash
./codegen/gen_sqlc.sh
```

Runs `sqlc generate -f codegen/sqlc.yaml` from the project root.

## Adding a Table — checklist

1. Write a goose migration in `versions/`.
2. `./db_migrations/check.sh` — validate the new file (duplicate timestamps, goose syntax).
3. `./db_migrations/migrate.sh up`
4. Regenerate code: `./codegen/gen_jet.sh` or `./codegen/gen_sqlc.sh`.
5. Add the aggregate adapter in `internal/adapter/repo/` (`xxx_repo.go`, `xxx_mapper.go`) and the matching domain types in `internal/domain/`. Follow `.claude/rules/persistence.md` and `.claude/rules/domain.md`.
