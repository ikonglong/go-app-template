# Architecture & Project-Wide Conventions

**Scope:** entire project (this file loads in every session).

Path-scoped coding guides live in `.claude/rules/` and load automatically
when Claude works with matching files:

| Rule | Applies to |
|---|---|
| `.claude/rules/domain.md` | `internal/domain/**/*.go` |
| `.claude/rules/persistence.md` | `internal/adapter/out/db/**/*.go` |
| `.claude/rules/codegen.md` | `db-migrations/**` |

## Layout

Hexagonal (ports-and-adapters):

```
internal/
├── domain/          aggregates, value objects, ports (interfaces), sentinel errors
├── application/     use cases (services that orchestrate domain + ports)
└── adapter/
    ├── in/          inbound adapters (HTTP / CLI / gRPC handlers)
    └── out/         outbound adapters (DB, message bus, external APIs)
```

Dependency direction: `application → domain`, `adapter → domain`, `adapter → application`. Domain depends on nothing inside the project; in particular, **domain does not import anything under `adapter/`** and never references go-jet, `database/sql`, HTTP, or time-of-day side effects.

## Naming Conventions

These are **project conventions**, not Go idioms. Apply them consistently.

| Rule | Example | Note |
|---|---|---|
| `I` prefix on every interface | `IAccountRepo`, `IRepo[T]`, `IMapper[D, R]`, `IVisitable[V]` | Deliberately against Go's idiomatic style (`Reader`/`Writer`). The team chose explicit prefix; do not drop it. |
| Implementation types are exported | `AccountRepo`, `AccountMapper` | Constructors return the concrete type, not the interface (Go's "accept interfaces, return structs"). |
| Compile-time interface checks | `var _ domain.IAccountRepo = (*AccountRepo)(nil)` | Adds the assertion that's lost when constructors return concrete types. Place at the file bottom. |
| Sentinel errors per aggregate | `ErrAccountNotFound = apperror.NewNotFound(...)` | Callers branch with `errors.Is`. Built via `github.com/ikonglong/go-apperror` factories so the sentinel carries a `Code` (+ optional `Case`). |

## Working Style

- **Simple first.** Fewest lines that solve the actual task. No speculative abstraction, no unused parameters, no "we might need this later" code paths. Three repeated lines beat a premature generic.
- **Plan before non-trivial work.** Architectural decisions, mass renames, destructive ops — confirm first.
- **Comments are a contract.** When you rename a type, update every doc comment that mentions it. When a future-tense comment ("we will eventually do X") is fulfilled, sweep it. Stale comments are worse than no comments.
- **Error handling at boundaries only.** Don't validate against scenarios that can't happen inside the project. Trust internal callers; validate user input at the inbound adapter.

## Coding Guide

- **Error handling**: @error_handling_guide.md
