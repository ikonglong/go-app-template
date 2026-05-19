---
paths:
  - "db-migrations/**"
---

# DB Migrations & Code Generation

## Migrations: `migrate.sh`

Wraps [goose](https://github.com/pressly/goose). Reads connection from `.env` (`DATABASE__*` keys) and forwards remaining args to `goose`.

```bash
./db-migrations/migrate.sh status
./db-migrations/migrate.sh up
./db-migrations/migrate.sh create add_orders_table   # writes to versions/
```

Migration files: `versions/<timestamp>_<name>.sql` — goose-format with `-- +goose Up` / `-- +goose Down` sections.

## Code Generation: `gen.sh`

Wraps the [go-jet](https://github.com/go-jet/jet) generator. Same `.env` loading; everything after the script name passes through to `jet`.

```bash
./db-migrations/gen.sh                  # default: full schema, skip goose_db_version
./db-migrations/gen.sh -tables=account  # allow-list mode
./db-migrations/gen.sh -schema=auth     # any jet flag pass-through; later values win
./db-migrations/gen.sh -h               # show injected defaults
```

### Defaults injected

```
-source=postgres
-dsn=<built from .env>
-schema=public
-path=./internal/adapter/out/db/gen
-rel-model-path=./record           ← package name becomes "record" not "model"
-ignore-tables=goose_db_version    ← suppresses the migration tracking table
```

### Gotcha: `-tables` and `-ignore-tables` are mutually exclusive

`jet` rejects them together with an explicit error. `gen.sh` detects user-supplied `-tables` and drops the `-ignore-tables` default automatically. **Don't pass both manually.**

## Adding a Table — checklist

1. Write a goose migration in `versions/`.
2. `./db-migrations/migrate.sh up`
3. `./db-migrations/gen.sh` — picks up the new table automatically; no edits to the script needed.
4. Add the aggregate adapter in `internal/adapter/out/db/` (`xxx_repo.go`, `xxx_mapper.go`) and the matching domain types in `internal/domain/`. Follow `.claude/rules/persistence.md` and `.claude/rules/domain.md`.
