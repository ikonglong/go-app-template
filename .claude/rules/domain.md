---
paths:
  - "internal/domain/**/*.go"
---

# Domain Layer Coding Guide

## Aggregates

`Account` is the canonical example.

### Field Visibility

All struct fields are unexported. Read access is split:

- **`func (*Account) ID() string`** is the **only** exported getter. Identity is special: persistence, logging, and cross-aggregate references all need it, and it never changes during the aggregate's lifetime.
- **All other state goes out via the visitor protocol** (`Accept` / `IAccountVisitor`). Callers can't depend on the internal layout, which leaves room to wrap fields in value objects later without breaking call sites.

### Two Factories per Aggregate

Every aggregate has **exactly two constructors**, with deliberate semantic distinction:

| | `CreateAccount` | `RebuildAccount` |
|---|---|---|
| Lifecycle event | **Birth** of a fresh aggregate | **Resumption** of an existing one |
| Caller | application/domain code (signup, admin invite, bootstrap) | persistence layer (mapper, snapshot replay, test fixture) |
| Invariant validation | future home for it | skipped — data already cleared the schema/domain checks on its way to storage |
| Server-managed fields | computed by the factory (e.g. `emailVerified=false`, single `now` for both timestamps) | passed in verbatim from storage |
| Frequency | exactly **once per aggregate identity** in the system's history | every time we hydrate the aggregate from storage |

Calling `CreateAccount` twice with the same ID is a bug: it creates one identity twice rather than starting an independent lifecycle. This is never a supported path. The "I have all the bytes back, give me the object" path is `RebuildAccount`.

### Naming Rules for Repos and Factories

- **Repository methods use `Add` / `Update` / `Delete`** (DDD collection-style). Do not name the repo's insert method `Create` — that's reserved for the domain factory.
- **Domain factories are `CreateXxx` / `RebuildXxx`** as above. `New` is reserved for adapter-side constructors (`NewAccountRepo`, `NewAccountMapper`).
- **`Rebuild` not `Reconstitute`.** Same DDD concept; chosen for readability.
- **`MustGet` vs `FindByID`.** `Must*` is the Go convention for "panic on failure". Do not name a function `Must*` if it returns an error; do not name a panicking function `Find*`.

## Visitor Protocol

```go
// internal/domain/visitor.go
type IVisitable[V any] interface {
    Accept(visitor V)
}

// internal/domain/account.go
type IAccountVisitor interface {
    ID(string)
    Name(string)
    Email(*string)
    // ... one method per field
}

type DefaultAccountVisitor struct{}     // no-op base, all 11 methods
```

### When to embed `DefaultXxxVisitor`

Embed it when the visitor **legitimately ignores most fields**: DTO builders that need a subset, redaction visitors that touch only sensitive columns, partial JSON output, patch detectors.

### When NOT to embed it

When the visitor must cover every field (e.g. a full-fidelity persistence mapper). Embedding the no-op default opts out of the compiler's "every field handled" check — a new field added to the visitor interface will silently fall through to the default. In those cases, implement every method explicitly so a forgotten one is a build error.

## Repository Ports

Generic CRUD port:

```go
// internal/domain/repo.go
type IRepo[T any] interface {
    Add(ctx context.Context, e *T) error
    Update(ctx context.Context, e *T) (int64, error)        // rowsAffected, real error
    Delete(ctx context.Context, id string) (int64, error)
    FindByID(ctx context.Context, id string) (*T, error)    // (nil, nil) = not found
    MustGet(ctx context.Context, id string) *T              // panics on err or absence
}
```

Aggregate-specific port embeds it and adds finders:

```go
// internal/domain/account.go
type IAccountRepo interface {
    IRepo[Account]
    FindByEmail(ctx context.Context, email string) (*Account, error)
    FindByPhone(ctx context.Context, phone string) (*Account, error)
    FindByProvider(ctx context.Context, provider, providerUserID string) (*Account, error)
}
```

### Error / outcome contract

| Method | Absence | Operational failure |
|---|---|---|
| `FindXxx`, `FindByID` | `(nil, nil)` — a normal lookup outcome, **not** an error | `(nil, err)` |
| `Update`, `Delete` | `(0, nil)` — caller decides what zero rows means | `(0, err)` |
| `MustGet` | **panics** (absence is a programmer-error contract violation here) | **panics** with the error |

The repo deliberately stays **semantically neutral** about absence:

- Finders separate "no row" (`nil` pointer) from "operational failure" (`err`). Callers do a nil check; they do NOT need `errors.Is(err, ErrXxxNotFound)` to disambiguate. This avoids mixing "not found" (normal flow control) with "DB is down" (real failure) in the same `err != nil` branch.
- `Update` / `Delete` return rows-affected verbatim. Zero rows means different things in different callers — idempotent delete, conditional update under optimistic locking, batch cleanup with no matches today, target genuinely missing — and the repo can't know which. The caller branches on the int64 if it cares.
- The aggregate's `ErrXxxNotFound` sentinel is **not** returned by the repo. It still exists for **application / handler code** to use as a prebuilt "this should have existed" signal (e.g. a `GET /accounts/{id}` endpoint that maps a `(nil, nil)` finder result to `404 ErrAccountNotFound`), so the response builder doesn't have to reconstruct the same `apperror.NewNotFound(...)` at every site.

`MustGet` is for callers that have already established the ID exists (e.g. derived from a freshly issued FK in the same transaction). Default to `FindByID` everywhere else.
