# go-app-template

An application template that demonstrates the architecture and framework. By design it carries only thin, simple business features — but the applications and services built from it as their prototype will not. It exists precisely to give such complex, feature-rich applications a genuinely useful starting point.

So judge every design and implementation decision by the standard of the complex production app this seeds — never by the template's current simplicity. Few or simple features today is no license for crude or stopgap solutions; build it the way the real, complex application would need.

None of this contradicts YAGNI. YAGNI — the rule of three, and not designing for hypothetical futures — applies in full to the real applications grown from this template, where features arrive organically over time. It does not govern the template itself. The template is a proving ground that, by its nature, has only one or two call sites; read literally, the rule of three would dismiss its canonical designs as premature abstraction. But the abstraction is not speculative here — the pattern *is* the deliverable — so model it in full rather than wait for a third use that never comes.

## Project Overview

### Architecture

**Style**: Hexagonal (ports & adapters). Full spec: @architecture.md

### Layout

Tier-consistent flat layout: **every top-level dir under `internal/` is an
architectural tier, not a capability.** Capabilities live one level down under
`adapter/<capability>/`. Dependencies always flow inward toward `domain/`.

```
cmd/server/          Binary entry. Thin: Config -> logger -> BuildContainer -> Invoke(Run).
internal/
  domain/            Domain core: entities, value objects, aggregates, repo ports, sentinel errors.
                     Imports nothing internal.
  application/       Use cases, command-oriented (Fowler COI): <Verb>Cmd with
                     Run(ctx, in) (out, error). Holds application logic (orchestration);
                     delegates domain logic to domain.
  common/            Foundation: business-agnostic utils + interfaces (clock, idgen, log).
                     Importable by anyone; imports nothing internal.
  adapter/           Adapter tier. One Go package per capability.
    rest/            Driving (inbound): Hertz HTTP handlers, error->HTTP mapping, middleware.
    repo/            Driven (outbound): persistence (DDD Repository). jet/ + sqlc/ are
                     parallel impls of the same ports; pick one per binary.
    <capability>/    Driven (outbound): other capability (e.g. passwordhash)
  assembly/          Assembly root (outside the hexagon): dig wiring, Config, lifecycle.
                     One module_<capability>.go per capability.
  infra/             OPTIONAL (absent today): self-authored client code; no port knowledge.
db-migrations/       SQL migrations + scripts (ops, outside the hexagon).
docs/                Design notes.
.claude/             Claude Code config: project instructions, architecture & coding guides, path-scoped rules.
```

### Dependency rules

- `domain/` and `application/` MUST NOT import `adapter/*` or `infra/`.
- `domain/` references no infrastructure at all — not go-jet, `database/sql`, HTTP, or
  time-of-day side effects (wall-clock access goes through the `clock` port).
- `infra/` MUST NOT import `domain/` or `application/` — it is pure client code, unaware
  that ports exist.
- Adapters are the only place ports meet clients (they import a port + an SDK/infra).
- Direction is read from the package name (`rest` = driving; `repo`, `passwordhash` =
  driven). No `in/` vs `out/` grouping inside `adapter/`.
- New outbound capability -> `adapter/<capability>/`, named by capability not vendor.
  Multiple coexisting impls -> sub-package `adapter/<capability>/<impl>/`.

## Working Discipline

- **Plan before non-trivial work.** Architectural decisions, mass renames, destructive ops — confirm first.

## Coding Guides

### Naming

These are **project conventions**, not Go idioms. Apply them consistently.

| Rule | Example | Note |
|---|---|---|
| `I` prefix on every interface | `IAccountRepo`, `IRepo[T]`, `IMapper[D, R]`, `IVisitable[V]` | Deliberately against Go's idiomatic style (`Reader`/`Writer`). The team chose explicit prefix; do not drop it. |
| Implementation types are exported | `AccountRepo`, `AccountMapper` | Constructors return the concrete type, not the interface (Go's "accept interfaces, return structs"). |
| Compile-time interface checks | `var _ domain.IAccountRepo = (*AccountRepo)(nil)` | Adds the assertion that's lost when constructors return concrete types. Place at the file bottom. |
| Sentinel errors per aggregate | `ErrAccountNotFound = apperror.NewNotFound(...)` | Callers branch with `errors.Is`. Built via `github.com/ikonglong/go-apperror` factories so the sentinel carries a `Code` (+ optional `Case`). |

- **Domain concept first.** Lead a type / file name with the full noun phrase that names the concept (`HealthCheck`, `AccountRegistration`), not a truncation that loses it (`Health`). The role suffix (`Handler` / `Repo` / `Mapper` / …) is optional and follows the two rules below. This overrides Go's "shortest readable name" instinct when the short form drops the concept.
- **File names — bare concept by default; add the role suffix only to disambiguate siblings in the same directory.** `domain/account.go`, `rest/health_check.go` (bare); `repo/jet/account_repo.go` + `account_mapper.go` (suffixed, because they coexist). New file → no name collision in the dir → bare; collision → give both the suffix.
- **Type names — keep the role suffix.** `HealthCheckHandler`, not `HealthCheck`: cross-package use (`rest.HealthCheckHandler`) and sibling DTOs (`HealthCheckResp`) need the disambiguation.
- **Adapter files & types.** Name by the port; bring in the algorithm / vendor only when several impls coexist.
  - One impl: file = `snake_case` of the interface minus the leading `I` (`IPasswordHasher` → `password_hasher.go`); type = the interface minus `I` (`PasswordHasher`).
  - Several impls in one package: file = algorithm / vendor (`bcrypt.go`, `argon2.go`); type = algorithm / vendor + interface minus `I` (`BcryptPasswordHasher`, `Argon2PasswordHasher`). For `repo/` this is the `jet/` + `sqlc/` sub-package split, files inside named aggregate + role.
- **Abbreviations**: in type / var / file names, prefer `Req` / `Resp` over `Request` / `Response` (e.g. `signUpReq`, `accountResp`, `errorResp`, `error_resp.go`). The full form is reserved for stdlib / framework types we don't own (`http.Request`, `app.RequestContext`).

### Comments

- **Comments are a contract.** When you rename a type, update every doc comment that mentions it. When a future-tense comment ("we will eventually do X") is fulfilled, sweep it. Stale comments are worse than no comments.
- **Doc comments say what, not how.** A type / function / method doc comment describes what it does and its contract — not its implementation.
- **No noise comments.** Don't restate what the code already makes obvious; a comment that just paraphrases self-describing code is clutter.
- **Put intent next to the code.** When a block's background or intent is non-obvious, comment it directly above that block, not in the enclosing function / method doc comment.

### Error handling

@error_handling_guide.md

### Logging

The guides below are **general** logging references (levels + worked examples); their examples use **unstructured** logging (format-string pseudocode). Take the level choices and semantics from them — but this app-template logs **structured** via `log/slog` (`internal/common/log/`), so emit through that logger's API and follow structured-logging best practices (key/value attributes, not string interpolation).

- **Levels & best practices**: @logging-best-practices.md
- **What each level captures**: @logging-examples-by-level.md
- **All levels in one function (production style)**: @logging-unified-example.md
- **State-machine transitions**: @logging-state-machine-example.md

### Request and internal args validation

Validation is layered — guard hard at the boundary so the inside can relax:

- **Interface layer (inbound `xxxReq`)** — the strict gate. Validate external input against the API IDL contract: presence, types, value ranges, formats (structural / value checks only). Every inbound `xxxReq` type carries its own `func (r *xxxReq) validate() error`; handlers call it right after `BindJSON` and route a non-nil error through `renderError`. Holds even for single-rule validators — consistency over a few saved lines.
- **Inside the app** — the boundary already guaranteed well-formed input, so don't re-check the same args layer by layer. Trust internal callers; avoid over-defensive validation.
- **Domain layer** — business rules and invariants live here; checks that need IO (e.g. uniqueness) are orchestrated by the application service.

### Database migrations

Migration scripts must be **safe**, **idempotent**, and **non-blocking** (assume large production tables, 100M+ rows), and every downgrade must cleanly undo its upgrade. Full guidelines: @db-migrations.md.
