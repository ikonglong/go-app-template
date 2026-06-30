# Error Handling Guide (for Claude Code)

This guide is the **Go + `go-apperror` realization** of the universal framework
in `error-handling-best-practices.md`. It applies when writing or modifying a Go
application that:

1. Adopts the ports-and-adapters architecture in `architecture.md`
   (Interfaces / Application / Domain layers, with Driven Adapters and optional
   Infrastructure).
2. Uses `github.com/ikonglong/go-apperror` as its error model.

**Read the base first.** `error-handling-best-practices.md` owns the universal
model and decisions — the origin categories, the *construct-inward /
translate-at-the-boundaries* principle, per-layer responsibilities, dependency
propagation, and the retry / logging / sub-case / anti-pattern guidance. This
guide does **not** restate that reasoning; it gives the concrete Go realization:
the types, the per-Code factories, the idioms, and runnable code. Each section
points back to the base section whose principle it implements.

Read this guide before constructing, wrapping, propagating, logging, or mapping
any error. The rules here are normative — when this guide and a local convention
disagree, this guide wins; when this guide and the base disagree, the **base**
wins (per its own contract). Flag either conflict to the user.

**Type ⇄ origin-category map** (base → *Classifying an Error*):

| Base origin category | Concrete type |
|---|---|
| App-domain error | `*apperror.AppError` |
| Transport-layer failure | `*apperror.AppError` (`NewUnavailable` / `NewTimeout`, transport error in `WithCause`) |
| Remote-responded failure | `*apperror.AppError` wrapping the `*apperror.RemoteError` (remote response) in `WithCause` |

---

## 1. The two error types

### 1.1 `*apperror.AppError`

Realizes the base's **unified application error**. Fields (all accessed via
methods):

- `Code()` — one of the package's `Code` constants; the standardized failure
  taxonomy. **Use Code for cross-cutting decisions** (retry, log aggregation
  key, HTTP status mapping at the boundary).
- `Case()` — optional `Case` for a specific business condition
  (e.g. `"purchase_limit_exceeded"`). **Rare by default — see §3.6.**
- `Message()` — human-readable description for logs / responses.
- `Event()` — operation name in `"{namespace}.{operation}"` form
  (e.g. `"user.signup"`); used as the structured-log event name.
- `Details()` — ad-hoc structured details. One-off, low-frequency data only; for
  stable structured patterns use a typed wrapper struct.
- `Cause()` / `Unwrap()` — underlying cause for `errors.Is` / `errors.As`.
- `StackTrace()` — call stack captured at construction; rendered to logs only
  for unexpected (500-class) codes (§3.5), never to the client.

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
apperror.NewInternal("user.lookup",
    apperror.WithMessage("db query failed"), apperror.WithCause(err))
```

There is **no** generic `apperror.New(code, ...)` by design. For a
runtime-determined Code (rare; e.g. translating an HTTP status), write an
explicit switch in the calling code.

**Factories** (one per non-OK Code):

```
NewCancelled, NewUnknown, NewIllegalInput, NewTimeout,
NewNotFound, NewAlreadyExists, NewPermissionDenied, NewUnauthenticated,
NewTooManyRequests, NewFailedPrecondition, NewConflict, NewOutOfRange,
NewUnimplemented, NewInternal, NewUnavailable, NewIllegalState,
NewUnauthorized, NewIllegalArg
```

`NewTooManyRequests` and `NewConflict` cover gRPC's `RESOURCE_EXHAUSTED` and
`ABORTED` respectively — there are no separate `ResourceExhausted` / `Aborted`
factories.

### 1.2 `*apperror.RemoteError`

Realizes the base's **remote-error shape** (base → *Propagating Errors From
Dependencies*) for server-responded remote failures. A `RemoteError` is **not** a
subtype of `AppError` and carries no taxonomy of its own. At the call boundary the
driven adapter classifies the failure into our taxonomy and wraps the
`RemoteError` as the **cause** of a canonical `AppError` (via `WithCause`). That
`AppError` is the value that propagates; the `RemoteError` stays reachable as its
cause for forensics. Fields:

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

The base's **three "codes"** map to (the canonical one on the wrapping
`AppError`, the other two on the `RemoteError`):

```
appErr.Code()          // our taxonomy — retry / circuit breaker / log aggregation
remoteErr.StatusCode() // protocol HTTP/RPC status — transport-class retry decisions
remoteErr.BodyCode     // remote's own code string — forensics / runbook / label
```

**Rules — apply wherever you build or read a RemoteError:**

- **Wrap, don't replace.** Build the `RemoteError`, then wrap it as the cause of
  a canonical `AppError` carrying the right `Code`:
  `apperror.NewNotFound("svc.op", apperror.WithCause(remoteErr))`. Propagate that
  `AppError`; a `RemoteError` must never be the top-level error past the adapter.
- **Recover the canonical view with `errors.As(err, &appErr)`** — because the
  `AppError` wraps the `RemoteError`, it is the layer found first. There is no
  separate `Canonical` accessor.
- **Recover the remote-side root cause with `errors.As(err, &remoteErr)`** — a
  centralized boundary logger uses it to record `Service` / `Operation` /
  `StatusCode` / `BodyCode`. Layers above the adapter branch on the `AppError`,
  not the `RemoteError`.

**Service-specific envelopes** (e.g. Stripe-shaped errors): embed `*RemoteError`
in a wrapper type; consumers reach the extra fields via `errors.As`:

```go
type StripeError struct {
    *apperror.RemoteError
    DeclineCode    string
    DeclineMessage string
}
```

### 1.3 Optional: numeric Case identifiers

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
err := apperror.NewNotFound("user.lookup",
    apperror.WithMessage("user not found"), apperror.WithCase(c))
// c.Identifier() == "1_3_1042"
```

For most apps, descriptive string cases via `apperror.NewStrCase("...")` are
simpler and preferred.

---

## 2. Per-layer realization

The responsibilities are defined in the base → *Per-Layer Responsibilities* and
*Construct Inward, Translate at the Boundaries*. Below is the Go code for each.

### 2.1 Domain layer

Produces **pure domain errors** as `AppError`. Typical codes: `CodeNotFound`,
`CodeAlreadyExists`, `CodeFailedPrecondition`, `CodeOutOfRange`,
`CodeIllegalState`, `CodeIllegalArg`. Never builds a `RemoteError`; never uses
transport/protocol codes (`CodeCancelled`, `CodeTimeout`, `CodeUnavailable`,
`CodeTooManyRequests`, `CodeUnauthorized`).

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

### 2.2 Application layer

Usually **propagates** errors from domain and driven adapters; sometimes
constructs its own `AppError` for use-case-level preconditions. Driven adapters
already translate a remote failure into a canonical `AppError` (with the
`RemoteError` wrapped as its cause — §2.3), so the application layer normally just
lets that `AppError` propagate.

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
            appErr.AddNote("during user.signup")   // context on the SAME error (§3.1, §3.3)
        }
        return err
    }
    if err := s.users.Insert(ctx, user); err != nil {
        return err   // pass through; Insert is a driven-adapter call
    }
    return nil
}
```

### 2.3 Driven adapters (external service clients)

Where `RemoteError` is constructed and where remote → canonical translation
lives. Every driven adapter owns its translation function.

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
            return nil, apperror.NewInternal(
                "user-service.GetUser",
                apperror.WithMessage("decoding user-service response"),
                apperror.WithCause(err),
            )
        }
        return &u, nil
    }

    // ── Server responded with failure: build a RemoteError, then wrap it as
    //    the cause of a canonical AppError ──
    var envelope struct {
        Code    string `json:"code"`
        Message string `json:"message"`
    }
    _ = json.Unmarshal(body, &envelope) // best-effort; missing fields OK

    remoteErr := &apperror.RemoteError{
        Service:     "user-service",
        Operation:   "GetUser",
        Response:    &apperror.Response{StatusCode: resp.StatusCode, Body: body},
        BodyCode:    envelope.Code,
        BodyMessage: envelope.Message,
        RetryAfter:  parseRetryAfter(resp.Header.Get("Retry-After")),
    }
    return nil, translateUserServiceError(remoteErr)
}

// Each driven adapter owns its translation: it classifies the remote failure
// into our taxonomy and wraps the RemoteError as the canonical AppError's cause.
// Body code wins when present; status is the fallback.
func translateUserServiceError(re *apperror.RemoteError) *apperror.AppError {
    ev := re.Event() // "user-service.GetUser"
    status := re.StatusCode()
    cause := apperror.WithCause(re)
    switch re.BodyCode {
    case "USER_GONE":
        return apperror.NewNotFound(ev, apperror.WithMessage("user not found"), cause)
    case "RATE_LIMITED":
        return apperror.NewTooManyRequests(ev, apperror.WithMessage("rate limited by user-service"), cause)
    }
    switch {
    case status == 404:
        return apperror.NewNotFound(ev, apperror.WithMessage("user not found"), cause)
    case status == 429:
        return apperror.NewTooManyRequests(ev, apperror.WithMessage("rate limited by user-service"), cause)
    case status == 401:
        return apperror.NewUnauthenticated(ev, apperror.WithMessage("not authenticated to user-service"), cause)
    case status == 403:
        return apperror.NewPermissionDenied(ev, apperror.WithMessage("forbidden by user-service"), cause)
    case status >= 500:
        return apperror.NewUnavailable(ev, apperror.WithMessage("user-service error"), cause)
    default:
        return apperror.NewUnknown(ev, apperror.WithMessage("user-service call failed"), cause)
    }
}
```

Pick the canonical `Code` so cross-cutting middleware behaves correctly (retry on
`CodeUnavailable` / `CodeTimeout` / `CodeTooManyRequests`). Default HTTP mapping
can start from `apperror.OpCodeFor(statusCode)`, but most adapters need
service-specific overrides for in-body codes.

### 2.4 Interfaces (driving adapters)

Maps an `AppError`'s `Code` to the HTTP status; a `RemoteError` or any other
non-AppError arriving *unconverted* falls back to 500. Logs the full error;
sanitizes the response.

```go
func renderError(w http.ResponseWriter, r *http.Request, err error) {
    // errors.As recovers the canonical *AppError — including when a RemoteError
    // is wrapped as its cause (the adapter's translation, §2.3), since the
    // AppError is the outer layer. Only a truly non-AppError reaching here (a bug
    // or an untranslated infrastructure error) carries no canonical Code, so it
    // falls back to 500. (The full error is still logged below, with a
    // RemoteError's remote fields intact.)
    var appErr *apperror.AppError
    if !errors.As(err, &appErr) {
        appErr = apperror.NewInternal("rest.unhandled", apperror.WithCause(err))
    }

    httpStatus, ok := apperror.HTTPStatusFor(appErr.Code())
    if !ok {
        httpStatus = apperror.StatusInternalServerError
    }

    // Log the full error — ErrAttrs/ErrGroup still surfaces a RemoteError's
    // service / operation / status / canonical-code for diagnosis (§3.5).
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

## 3. Idioms

Go realizations of the base's *Construct Inward* and *Cross-Cutting Concerns*.

### 3.1 Add context as an error propagates up

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

### 3.2 Wrap a non-AppError

When you catch an `error` that is NOT an AppError (third-party, stdlib, generic
transport call), wrap it with a factory + `WithCause` (which preserves it for
`errors.Is` / `errors.As`):

```go
data, err := json.Marshal(payload)
if err != nil {
    return apperror.NewInternal(
        "checkout.serialize_request",
        apperror.WithMessage("marshalling payload"),
        apperror.WithCause(err),
    )
}
```

### 3.3 `AddNote` vs `fmt.Errorf %w` vs factory

These three Go tools realize the base's "adding context as it rises" table:

| Goal | Tool |
|---|---|
| Same error, more context, keep `*AppError` type | `appErr.AddNote("...")` |
| New error event, different semantics, grow chain | `fmt.Errorf("ctx: %w", err)` |
| Wrap a non-AppError into an AppError | factory + `WithCause(err)` |

### 3.4 Retry policy

Realizes base → *Cross-Cutting Concerns / Retry policy*. Branch on the canonical
`Code` — `errors.As(err, &ae)` recovers it whether the failure is a plain
`AppError` or one wrapping a `RemoteError` (§1.2). When a `RemoteError` is in the
chain, pull its `RetryAfter` hint via a second `errors.As`:

```go
func shouldRetry(err error) (bool, time.Duration) {
    var ae *apperror.AppError
    if !errors.As(err, &ae) {
        return false, 0
    }
    var retryAfter time.Duration
    if re := (*apperror.RemoteError)(nil); errors.As(err, &re) {
        retryAfter = re.RetryAfter
    }
    switch ae.Code() {
    case apperror.CodeTooManyRequests, apperror.CodeUnavailable:
        return true, retryAfter
    case apperror.CodeTimeout:
        return true, 0
    }
    return false, 0
}
```

### 3.5 Structured logging

Realizes base → *Cross-Cutting Concerns / Structured logging*. In this project
the fields and stack come from `applog.ErrAttrs` / `applog.ErrGroup`, and the
`event` from the level wrappers `applog.ErrorAttrs(ctx, event, msg, …)`. The
field set the base defines maps to:

```
event        = appErr.Event()            // exactly one event key
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

`stack` is a `[][]string` with no embedded newlines, so it never splits a log
record; it's rendered only for unexpected (500-class) codes
(`Internal`, `Unknown`, `IllegalState`), and a `RemoteError` chain
emits none. **Do not** log raw `Response.Body` / `Request.Body` unconditionally —
redact at the boundary.

### 3.6 When to define a specific Case

The rationale and threshold live in base → *Cross-Cutting Concerns / When to
define a specific sub-case*. **Default: don't.** Add a `Case` only when a
concrete consumer will branch on it.

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

**Pure noise** — a Case mirroring an internal failure mode no caller acts on:

```go
// Bad — cardinality with no consumer.
apperror.NewIllegalInput("user.signup",
    apperror.WithMessage("email must contain @"),
    apperror.WithCase(apperror.NewStrCase("email_missing_at")))
```

---

## 4. Anti-patterns — DO NOT

The base → *Anti-Patterns* lists the universal traps. These are the
**`go-apperror`-specific** ones:

1. **Do NOT construct an AppError with `CodeOK`.** No factory exists; CodeOK is
   only for the Code↔HTTP-status mapping.
2. **Do NOT mint your own `Code` values** (e.g. `apperror.Code(100)`). The
   taxonomy is closed; if nothing fits, reconsider the semantic (usually the
   closest Code + a specific Case).
3. **Do NOT add methods to `AppError` that forward to remote-specific data.**
   AppError is the domain-error type; remote specifics belong on RemoteError.

(And the universal ones, restated in Go terms: don't put the failure mode in
`event`; don't use `\n` in messages — use `AddNote`'s `" -> "` separator.)

When in doubt, re-read the base's *Classifying an Error* and *Construct Inward,
Translate at the Boundaries* — most mistakes start there.
