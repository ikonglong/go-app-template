# Error Handling Guide (for Claude Code)

This guide tells you, Claude Code, how to handle errors when writing or
modifying a Go application that:

1. Adopts the ports-and-adapters architecture described in `architecture.md`
   (Interfaces / Application / Domain layers, with Driven Adapters and optional
   Infrastructure).
2. Uses the `github.com/ikonglong/go-apperror` library as its error model.

Read this guide before constructing, wrapping, propagating, logging, or mapping
any error. The rules here are normative — when a rule and a local convention
disagree, the rule wins; flag the conflict to the user.

---

## 1. TL;DR

**Every error is one of three categories — the category picks the type:**

| Category | Cause | Type to use |
|---|---|---|
| **App-domain error** | Validation, business-rule violation, broken internal invariant — originating in our app. | `*apperror.AppError` |
| **Transport-layer failure** | We called a remote service and **no response was received** (DNS failed, TCP refused/reset, TLS handshake failed, timed out before any bytes arrived). | `*apperror.AppError`, usually `NewUnavailable` / `NewTimeout`, transport error in `WithCause`. |
| **Remote-service-responded failure** | We called a remote service and **the server returned a response** (any status code, in-band error envelope, etc.). | `*apperror.RemoteError` |

A transport failure is an `AppError`, **not** a `RemoteError` — a `RemoteError`
exists only when a response was received.

**Construct inward, translate only at the boundaries.** Errors are *constructed*
in the inner layers (domain, application) and propagate outward largely
untouched — annotate with `AddNote` / `%w`, but don't translate. Actual handling
happens only at the two boundaries: the outbound driven adapter translates
external failures into our error model (§3.3), and the inbound interface layer
translates our errors into the wire response, logging and sanitizing what
reaches the client (§3.4).

**Decision tree — which type / code:**

```
Did the error happen inside our app's logic (no remote call involved)?
├─ YES → AppError, factory chosen by the failure semantics (§3.1, §3.2).
└─ NO → We were calling a remote service.
        ├─ Did the server send back a response?
        │   ├─ NO  → AppError. NewUnavailable for refused/reset/DNS,
        │   │        NewTimeout for context deadline; WithCause(transportErr).
        │   └─ YES → RemoteError, built by the driven adapter with a
        │            Canonical *AppError + Response + BodyCode/BodyMessage (§2.2, §3.3).
        └─ Response indicates success? → return the parsed result; not an error.
```

**Per-layer cheat-sheet:**

```
DOMAIN LAYER       → AppError only; codes: NotFound, AlreadyExists,
                     FailedPrecondition, OutOfRange, IllegalState, IllegalArg
APPLICATION LAYER  → AppError (propagates or constructs); AddNote for
                     upstream context; no protocol translation
DRIVEN ADAPTER     → AppError for transport failures; RemoteError for
                     server-responded failures; owns translation:
                     remote signals → Canonical Code
INTERFACES LAYER   → errors.As to *AppError → map Code to status;
                     RemoteError / unknown → 500. Log full error; sanitize.

Construction       → per-Code factory(event, opts...) +
                     WithMessage / WithCase / WithCause / WithDetails
Propagation        → AddNote for same error; fmt.Errorf %w for new layer
Cross-cutting      → branch on appErr.Code() / remoteErr.Canonical.Code()
                     (check *RemoteError first — Canonical is not on the
                     errors.As chain)
```

---

## 2. The two error types

### 2.1 `*apperror.AppError`

The standardized application error. Fields (all accessed via methods):

- `Code()` — one of the package's `Code` constants; the standardized failure
  taxonomy. **Use Code for cross-cutting decisions** (retry, log aggregation
  key, HTTP status mapping at the boundary).
- `Case()` — optional `Case` for a specific business condition
  (e.g. `"purchase_limit_exceeded"`). **Rare by default — see §4.6.**
- `Message()` — human-readable description for logs / responses.
- `Event()` — operation name in `"{namespace}.{operation}"` form
  (e.g. `"user.signup"`); used as the structured-log event name.
- `Details()` — ad-hoc structured details. One-off, low-frequency data only; for
  stable structured patterns use a typed wrapper struct.
- `Cause()` / `Unwrap()` — underlying cause for `errors.Is` / `errors.As`.
- `StackTrace()` — call stack captured at construction; rendered to logs only
  for unexpected (500-class) codes (§4.5), never to the client.

**Construct only via per-Code factories**, all sharing one signature:

```go
func NewXxx(event string, opts ...Option) *AppError
```

- **`event`** (first positional, *required*) names the operation the failure
  occurred during, `"{namespace}.{operation}"` (e.g. `"user.signup"`,
  `"order.fulfilment.charge"`). Factories panic on empty event.
- **`WithMessage("...")`** attaches the message; optional — falls back to
  `Code.Description()` when omitted.
- **`WithCase(...)`**, **`WithCause(...)`**, **`WithDetails(...)`** layer the
  remaining optional fields.

```go
apperror.NewIllegalInput("user.signup", apperror.WithMessage("email must contain @"))
apperror.NewNotFound("user.lookup", apperror.WithMessage("user not found"))
apperror.NewInternalError("user.lookup",
    apperror.WithMessage("db query failed"), apperror.WithCause(err))
```

There is **no** generic `apperror.New(code, ...)` by design. For a
runtime-determined Code (rare; e.g. translating an HTTP status), write an
explicit switch in the calling code.

**Factories** (one per non-OK Code):

```
NewOpCancelled, NewUnknownError, NewIllegalInput, NewTimeout,
NewNotFound, NewAlreadyExists, NewPermissionDenied, NewUnauthenticated,
NewTooManyRequests, NewFailedPrecondition, NewOpConflict, NewOutOfRange,
NewUnimplemented, NewInternalError, NewUnavailable, NewIllegalState,
NewAuthorizationExpired, NewIllegalArg
```

`NewResourceExhausted` and `NewOpAborted` are deprecated aliases for
`NewTooManyRequests` and `NewOpConflict` — do **not** use them in new code.

### 2.2 `*apperror.RemoteError`

For server-responded remote failures. Fields:

- `Canonical *AppError` — **must be non-nil**. The driven adapter's translation
  of the *remote response* into our taxonomy, **preserving the remote error's own
  semantics** (remote HTTP 400 → `IllegalInput`, 404 → `NotFound`, …). It is a
  **parallel view, not a cause**, and it describes the *remote call*, not our own
  API contract: use it to *inspect* the failure (retry, circuit-breaking,
  logging), **not** as a ready-made response for our API clients (§3.4). Access it
  explicitly as `r.Canonical.Code()`.
- `Service`, `Operation` — logical names (e.g. `"user-service"`, `"GetUser"`),
  not an HTTP method+path.
- `Request *Request` — captured outbound request (optional, often nil to avoid
  logging sensitive bodies).
- `Response *Response` — **must be non-nil**. Captured response.
- `BodyCode`, `BodyMessage` — the remote's app-level error signals parsed from
  `Response.Body`; good low-cardinality observability keys.
- `RetryAfter time.Duration` — normalized retry hint, if the remote provided one.

Methods: `StatusCode()` (from `Response.StatusCode`) and `Event()`
(`Service + "." + Operation`).

**Three "codes" live on a RemoteError** — log all three as separate fields (§4.5):

```
r.Canonical.Code()  // our taxonomy — retry / circuit breaker / log aggregation
r.StatusCode()      // protocol HTTP/RPC status — transport-class retry decisions
r.BodyCode          // remote's own code string — forensics / runbook / label
```

**Rules — apply wherever you build or read a RemoteError:**

- **It is a leaf error**: no `Unwrap`, no `Cause`. `Canonical` is intentionally
  *not* on the `errors.Is` / `errors.As` chain.
- **Read the canonical view via `errors.As(err, &remoteErr)` then
  `remoteErr.Canonical`** — `errors.As(err, &appErr)` will NOT recover it. Always
  check `*RemoteError` before `*AppError`.
- **Do NOT set `WithCause` on `Canonical`** — a RemoteError has no cause (the
  response itself is the failure).
- **Do NOT set `WithDetails` on `Canonical`** — remote-side structured info goes
  in `BodyCode` / `BodyMessage` / `Response.Body`.
- **The Canonical's `event` is ignored** — `Event()` reads `Service.Operation`,
  never the embedded value. Pass a service-scoped placeholder like
  `"user-service.translate"`.

**Service-specific envelopes** (e.g. Stripe-shaped errors): embed `*RemoteError`
in a wrapper type; consumers reach the extra fields via `errors.As`:

```go
type StripeError struct {
    *apperror.RemoteError
    DeclineCode    string
    DeclineMessage string
}
```

### 2.3 Optional: numeric Case identifiers

If case identifiers must be numeric (e.g. stable API codes like `"1_3_1042"`),
use `apperror/numcase`:

```go
import "github.com/ikonglong/go-apperror/numcase"

factory, _ := numcase.NewCaseFactory(numcase.FactoryConfig{
    NumDigitsForAppCode:    1,
    NumDigitsForModuleCode: 1,
    NumDigitsForCaseCode:   4,
    CodeMapper:             numcase.NewDefaultCodeMapper(),
    AppCode:                1,
    ModuleCode:             3,
})
c, _ := factory.NewNotFound(1042)
err := apperror.NewNotFound("user gone", apperror.WithCase(c))
// c.Identifier() == "1_3_1042"
```

For most apps, descriptive string cases via `apperror.NewStrCase("...")` are
simpler and preferred.

---

## 3. Per-layer responsibilities

Four error-relevant layers, each with distinct rules. **Identify which layer
you're in before choosing how to handle an error.** The boundary principle from
§1 governs the split: inner layers construct and propagate; the two outer
boundaries translate.

### 3.1 Domain layer

Produces **pure domain errors** as `AppError`. Knows nothing about HTTP, RPC,
databases, or anything technical.

Typical codes: `CodeNotFound`, `CodeAlreadyExists`, `CodeFailedPrecondition`,
`CodeOutOfRange`, `CodeIllegalState` (invariant broken / data corruption),
`CodeIllegalArg` (programmer error: invalid args within our code).

```go
func (u *User) Promote() error {
    if u.tier == TierGold {
        return apperror.NewFailedPrecondition(
            "user.promote",
            apperror.WithMessage("user is already at top tier"),
            apperror.WithCase(apperror.NewStrCase("user_already_top_tier")),
        )
    }
    u.tier++
    return nil
}
```

**Forbidden in domain:** creating `RemoteError` (domain calls no remote
services); codes that imply transport/protocol concerns (`CodeOpCancelled`,
`CodeTimeout`, `CodeUnavailable`, `CodeTooManyRequests`,
`CodeAuthorizationExpired`).

### 3.2 Application layer

Usually **propagates** errors from domain and driven adapters; sometimes
constructs its own `AppError` for use-case-level preconditions.

**Converting a remote failure for our clients happens here.** A `RemoteError`
carries the *remote* call's semantics (§2.2), so decide per propagation strategy
how it should surface to our API consumers and wrap it into an `AppError`
accordingly — usually `InternalError` (a remote 400 is rarely our caller's
fault). Left unconverted, it reaches the boundary and renders as 500 (§3.4).

```go
func (s *SignupService) Signup(ctx context.Context, email string) error {
    if existing, err := s.users.FindByEmail(ctx, email); err == nil && existing != nil {
        return apperror.NewAlreadyExists(
            "user.signup",
            apperror.WithMessage("email already registered"),
            apperror.WithCase(apperror.NewStrCase("email_taken")),
        )
    }
    user, err := domain.NewUser(email)
    if err != nil {
        if appErr := (*apperror.AppError)(nil); errors.As(err, &appErr) {
            appErr.AddNote("during user.signup")   // context on the SAME error (§4.1, §4.3)
        }
        return err
    }
    if err := s.users.Insert(ctx, user); err != nil {
        return err   // pass through; Insert is a driven-adapter call
    }
    return nil
}
```

**Forbidden in application:** calling external HTTP/RPC directly, and
translating HTTP/RPC statuses to our `Code` — both belong in driven adapters.

### 3.3 Driven adapters (external service clients)

**Where `RemoteError` is constructed, and where remote → canonical translation
lives.** Every driven adapter owns its translation function. For the RemoteError
construction rules, see §2.2.

```go
func (c *UserServiceClient) GetUser(ctx context.Context, id string) (*User, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", c.base+"/users/"+id, nil)
    resp, err := c.http.Do(req)

    // ── Transport failure: AppError, NOT RemoteError ──
    if err != nil {
        return nil, apperror.NewUnavailable(
            "user-service.GetUser",
            apperror.WithMessage("user-service unreachable"),
            apperror.WithCause(err),
        )
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)

    // ── Success ──
    if resp.StatusCode < 400 {
        var u User
        if err := json.Unmarshal(body, &u); err != nil {
            return nil, apperror.NewInternalError(
                "user-service.GetUser",
                apperror.WithMessage("decoding user-service response"),
                apperror.WithCause(err),
            )
        }
        return &u, nil
    }

    // ── Server responded with failure: RemoteError ──
    var envelope struct {
        Code    string `json:"code"`
        Message string `json:"message"`
    }
    _ = json.Unmarshal(body, &envelope) // best-effort; missing fields OK

    return nil, &apperror.RemoteError{
        Canonical:   translateUserServiceError(resp.StatusCode, envelope.Code),
        Service:     "user-service",
        Operation:   "GetUser",
        Response:    &apperror.Response{StatusCode: resp.StatusCode, Body: body},
        BodyCode:    envelope.Code,
        BodyMessage: envelope.Message,
        RetryAfter:  parseRetryAfter(resp.Header.Get("Retry-After")),
    }
}

// Each driven adapter owns its translation. Body code wins when present; status
// is the fallback. (event is a placeholder — see §2.2.)
func translateUserServiceError(status int, bodyCode string) *apperror.AppError {
    const ev = "user-service.translate"
    switch bodyCode {
    case "USER_GONE":
        return apperror.NewNotFound(ev, apperror.WithMessage("user not found"))
    case "RATE_LIMITED":
        return apperror.NewTooManyRequests(ev, apperror.WithMessage("rate limited by user-service"))
    }
    switch {
    case status == 404:
        return apperror.NewNotFound(ev, apperror.WithMessage("user not found"))
    case status == 429:
        return apperror.NewTooManyRequests(ev, apperror.WithMessage("rate limited by user-service"))
    case status == 401:
        return apperror.NewUnauthenticated(ev, apperror.WithMessage("not authenticated to user-service"))
    case status == 403:
        return apperror.NewPermissionDenied(ev, apperror.WithMessage("forbidden by user-service"))
    case status >= 500:
        return apperror.NewUnavailable(ev, apperror.WithMessage("user-service error"))
    default:
        return apperror.NewUnknownError(ev, apperror.WithMessage("user-service call failed"))
    }
}
```

**Pick the Canonical Code so cross-cutting middleware behaves correctly** — e.g.
retry on `CodeUnavailable` / `CodeTimeout` / `CodeTooManyRequests`. Default HTTP
mapping can start from `apperror.OpCodeFor(statusCode)`, but most adapters need
service-specific overrides for in-body codes.

### 3.4 Interfaces (driving adapters)

Catches errors from Application, **translates them to the inbound protocol's wire
format** (HTTP response, gRPC status, queue NACK, …), logs, and sanitizes. It maps
an `AppError`'s `Code` to the protocol status; a `RemoteError` or any other
non-AppError that arrives *unconverted* falls back to 500 — the boundary can't
assume the remote's (or an unknown error's) semantics describe our API contract.
Converting a remote failure for our clients is the application layer's job (§3.2).

```go
func renderError(w http.ResponseWriter, r *http.Request, err error) {
    // Map only an *AppError's Code to a status. A RemoteError or any other
    // non-AppError reaching here was never converted by the application layer
    // for our clients, so its semantics don't describe our API contract —
    // fall back to 500. (The full error is still logged below, with a
    // RemoteError's remote fields intact.)
    var appErr *apperror.AppError
    if !errors.As(err, &appErr) {
        appErr = apperror.NewInternalError("rest.unhandled", apperror.WithCause(err))
    }

    httpStatus, ok := apperror.HTTPStatusFor(appErr.Code())
    if !ok {
        httpStatus = apperror.StatusInternalServerError
    }

    // Log the full error — ErrAttrs/ErrGroup still surfaces a RemoteError's
    // service / operation / status / canonical-code for diagnosis (§4.5).
    logError(r.Context(), err, appErr)

    // Sanitized response — Code / Case / Message only.
    body := errorResponseBody{Code: appErr.Code().Name(), Message: appErr.Message()}
    if c := appErr.Case(); c != nil {
        body.Case = c.Identifier()
    }
    renderJSON(w, int(httpStatus), body)
}
```

**Sanitize**: never put internal stack traces, raw transport errors, or
sensitive Request/Response bodies into the client response. Log them
server-side; expose only `Code`, `Case`, and a client-safe `Message`.

---

## 4. Common patterns

### 4.1 Add context as an error propagates up

`AddNote` adds upstream context to the SAME error's message (no new chain layer):

```go
err := repo.FindUser(id)
if err != nil {
    var appErr *apperror.AppError
    if errors.As(err, &appErr) {
        appErr.AddNote("loading user during checkout")
    }
    return err
}
```

The error stays the same `*AppError`; its Message is prepended with `" -> "`
separated context.

### 4.2 Wrap a non-AppError

When you catch an `error` that is NOT an AppError (third-party, stdlib, generic
transport call), wrap it with a factory + `WithCause` (which preserves it for
`errors.Is` / `errors.As`):

```go
data, err := json.Marshal(payload)
if err != nil {
    return apperror.NewInternalError(
        "checkout.serialize_request",
        apperror.WithMessage("marshalling payload"),
        apperror.WithCause(err),
    )
}
```

### 4.3 `AddNote` vs `fmt.Errorf %w` vs factory

| Goal | Tool |
|---|---|
| Same error, more context, keep `*AppError` type | `appErr.AddNote("...")` |
| New error event, different semantics, grow chain | `fmt.Errorf("ctx: %w", err)` |
| Wrap a non-AppError into an AppError | factory + `WithCause(err)` |

### 4.4 Retry policy

Branch on Canonical Code, falling back to RemoteError's protocol/body signals.
Check `*RemoteError` first — its Canonical is not on the `errors.As` chain (§2.2):

```go
func shouldRetry(err error) (bool, time.Duration) {
    var re *apperror.RemoteError
    if errors.As(err, &re) {
        switch re.Canonical.Code() {
        case apperror.CodeTooManyRequests, apperror.CodeUnavailable:
            return true, re.RetryAfter
        case apperror.CodeTimeout:
            return true, 0
        }
        return false, 0
    }
    var ae *apperror.AppError
    if errors.As(err, &ae) {
        switch ae.Code() {
        case apperror.CodeUnavailable, apperror.CodeTimeout:
            return true, 0
        }
    }
    return false, 0
}
```

### 4.5 Structured logging

Log these fields whenever you log an error (names are a recommended convention):

```
event        = the operation at log time, supplied by the caller
               (typically appErr.Event()) — exactly one event key
code         = appErr.Code().Name()
case         = appErr.Case().Identifier()    // if non-nil
message      = appErr.Message()
cause        = appErr.Cause().Error()        // if non-nil — SERVER-SIDE ONLY
// For a RemoteError, additionally:
service      = remoteErr.Service
operation    = remoteErr.Operation
status       = remoteErr.StatusCode()
body_code    = remoteErr.BodyCode
body_message = remoteErr.BodyMessage
retry_after  = remoteErr.RetryAfter
// For unexpected (500-class) codes, additionally:
stack        = captured call stack(s)        // SERVER-SIDE ONLY
```

`event` makes a great primary key; `code` / `case` / `body_code` are good
aggregation labels.

**`cause` and `stack` are server-side only** — never put them in the client
response (§3.4). `stack` is rendered only for unexpected, 500-class codes
(`InternalError`, `UnknownError`, `IllegalState`); routine errors omit it so
logs aren't buried in stack noise, and a `RemoteError` chain emits none (its
remote fields already locate the failure). It is a `[][]string` with no embedded
newlines, so it never splits a log record (§5).

**Do not** log raw `Response.Body` / `Request.Body` unconditionally — they may
contain sensitive data; redact at the boundary.

In this project these come from `applog.ErrAttrs` / `applog.ErrGroup` (error
fields + stack) and the level wrappers `applog.ErrorAttrs(ctx, event, msg, …)`
(which supply `event`).

### 4.6 When to define a specific Case

**Default: don't.** `Code` alone is enough most of the time, and an empty Case
keeps cardinality low and the API surface small.

**Add a Case only when a concrete consumer (UI screen, partner contract, runbook
step) will branch on it** — to show a friendlier prompt or trigger a different
flow. If nothing branches on it, don't mint it.

**Earns its keep** — signup `CodeAlreadyExists` where the UI swaps the generic
message for a password-recovery CTA when the conflict is on email/phone:

```go
if existing, _ := repo.FindByEmail(ctx, email); existing != nil {
    return apperror.NewAlreadyExists(
        "account.create",
        apperror.WithMessage("an account with this email already exists"),
        apperror.WithCase(apperror.NewStrCase("account_credential_taken")),
    )
}
```

Without the Case the client sees only `CodeAlreadyExists` — which could equally
be a duplicate username or idempotency key — and would have to parse `Message`.

**Pure noise** — a Case mirroring an internal failure mode no caller acts on:

```go
// Bad — cardinality with no consumer.
apperror.NewIllegalInput("user.signup",
    apperror.WithMessage("email must contain @"),
    apperror.WithCase(apperror.NewStrCase("email_missing_at")))
```

**Threshold, in order:** (1) Is there a concrete consumer that behaves
differently for this Case vs. a sibling under the same Code? (2) Would `Message`
alone be too brittle for it? Both yes → define it. Otherwise skip.

---

## 5. Anti-patterns — DO NOT

These are the traps not already stated as a positive rule above.

1. **Do NOT construct an AppError with `CodeOK`.** No factory exists; CodeOK is
   only for the Code↔HTTP-status mapping.
2. **Do NOT put the failure mode in `event`.** Event names the operation
   (`"user.signup"`); the failure mode belongs in `Code` / `Case`.
   `event="user.signup.email_invalid"` collapses two observability dimensions and
   breaks `event × code` pivots.
3. **Do NOT use `\n` in error messages.** Log aggregators split on newlines. Keep
   messages single-line; use `AddNote`'s `" -> "` for multi-layer context.
4. **Do NOT mint your own `Code` values** (e.g. `apperror.Code(100)`). The
   taxonomy is closed; if nothing fits, reconsider the semantic (usually the
   closest Code + a specific Case).
5. **Do NOT add methods to `AppError` that forward to remote-specific data.**
   AppError is the domain-error type; remote specifics belong on RemoteError.

When in doubt, re-read §1 — the categories, the boundary principle, and the
decision tree. Most mistakes start there.
