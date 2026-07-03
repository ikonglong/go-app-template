# Database Migrations

Guidelines for writing database migration scripts. Assume production databases with large tables (100M+ rows). Every migration must be **safe** — that is, **idempotent**, **non-blocking**, and **cleanly reversible** (the three Core Guidelines below).

> Scope: how to write **safe migration SQL**. For the tooling that authors, applies, validates, and regenerates code from migrations (`migrate.sh`, `upgrade.sh`, `check.sh`, `gen.sh`), see `.claude/rules/codegen.md`.

## Core Guidelines

1. **Idempotent** — Every statement must be re-runnable without side effects.
   - `ADD COLUMN IF NOT EXISTS`, `DROP COLUMN IF EXISTS`
   - `CREATE INDEX ... IF NOT EXISTS`, `DROP INDEX IF EXISTS`
   - `DROP CONSTRAINT IF EXISTS` before `ADD CONSTRAINT`

2. **Non-blocking** — Never hold an `ACCESS EXCLUSIVE` lock while scanning or rewriting data. Holding it is acceptable only for an instantaneous, metadata-only operation (e.g. a plain nullable `ADD COLUMN` with no default).
   - **CREATE/DROP INDEX** → use `CONCURRENTLY` (cannot run inside a transaction; must execute in autocommit mode). *(PostgreSQL-specific)*
   - **ADD COLUMN** → avoid `NOT NULL` without a default on existing large tables (rewrites the table).
   - **ADD CHECK/FK constraint** → always append `NOT VALID`, then `VALIDATE CONSTRAINT` separately if needed. `NOT VALID` skips scanning existing rows; without it, Postgres scans the entire table under an `ACCESS EXCLUSIVE` lock, blocking all reads and writes. *(PostgreSQL-specific)*
     - **CHECK**: usually no need to validate afterwards — existing rows already satisfy the old constraint, and the new constraint only widens the allowed values.
     - **FK**: run `VALIDATE CONSTRAINT` in a follow-up migration — existing rows may contain orphan references. `VALIDATE` holds only a `SHARE UPDATE EXCLUSIVE` lock (reads/writes continue; only other DDL is blocked). *(PostgreSQL-specific)*

3. **Downgrade must undo cleanly** — Mirror every upgrade action in reverse order.

