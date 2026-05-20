# Error Handling Guide (for Claude Code)

This guide tells you, Claude Code, how to handle errors when writing or
modifying a Go application that:

1. Adopts the ports-and-adapters architecture described in
   `architecture.md` (Interfaces / Application / Domain layers, with
   Driven Adapters and optional Infrastructure)
2. Uses the `github.com/ikonglong/go-apperror` library as its error model

Read this guide before constructing, wrapping, propagating, logging, or
mapping any error. The rules here are normative — when a rule and a
local convention disagree, the rule wins; flag the conflict to the user.

---

## 1. Mental model: three error categories

Every error in the app falls into exactly one category. **You must
identify which category an error is BEFORE writing code for it.**

| Category | Cause | Type to use |
|---|---|---|
| **App-domain error** | Validation, business rule violation, internal invariant broken, etc., originating in our app. | `*apperror.AppError` |
| **Transport-layer failure** | We tried to call a remote service and **no response was received** (DNS resolution failed, TCP refused/reset, TLS handshake failed, request timed out before any bytes arrived). | `*apperror.AppError`, typically `NewUnavailable` or `NewTimeout`, with the transport error attached via `WithCause`. |
| **Remote-service-responded failure** | We called a remote service and **the server returned a response** (any status code, in-band error envelope in the body, etc.). | `*apperror.RemoteError` |

Critical distinction: a transport failure is an `AppError`, **not** a
`RemoteError`. RemoteError's existence is conditioned on having received
a response.

---

## 2. Library types you'll use

### 2.1 `*apperror.AppError`

The standardized application error.

Fields (all accessed via methods):
- `Code()` — one of this package's `Code` constants. The standardized
  failure taxonomy. **Use Code for cross-cutting decisions** (retry,
  log aggregation key, HTTP status mapping at the boundary).
- `Case()` — optional `Case` identifying the specific business condition
  (e.g. `"purchase_limit_exceeded"`). **Rare by default** — add one only
  when a caller (UI, partner integration, runbook, dashboard) will branch
  on it to change user-facing behavior or trigger a different flow. See
  §4.6 for the threshold and a worked example.
- `Message()` — human-readable description for logs / responses.
- `Event()` — operation name in `"{namespace}.{operation}"` form
  (e.g. `"user.signup"`). Used as the structured-log event name.
- `Details()` — ad-hoc structured details. Only for one-off, low-frequency
  data; for stable structured patterns use a typed wrapper struct (see
  §6.4).
- `Cause()` / `Unwrap()` — underlying cause for `errors.Is` / `errors.As`.

**Construct only via per-Code factories**, which all share the same signature:

```go
func NewXxx(event string, opts ...Option) *AppError
```

- **`event`** is the first positional argument and is *required*. It
  names the operation the failure occurred during, in
  `"{namespace}.{operation}"` form (e.g. `"user.signup"`,
  `"order.fulfilment.charge"`). Factories panic on empty event so this
  can't be silently forgotten.
- **`apperror.WithMessage("...")`** attaches the human-readable message.
  Optional — when omitted, the message falls back to
  `Code.Description()` so the rendered error is still non-empty.
- **`apperror.WithCase(...)`**, **`apperror.WithCause(...)`**, and
  **`apperror.WithDetails(...)`** layer the remaining optional fields.

```go
apperror.NewIllegalInput("user.signup", apperror.WithMessage("email must contain @"))
apperror.NewNotFound("user.lookup", apperror.WithMessage("user not found"))
apperror.NewInternalError("user.lookup",
    apperror.WithMessage("db query failed"), apperror.WithCause(err))
```

There is **no** `apperror.New(code, ...)` generic constructor by design.
If you have a runtime-determined Code (rare; e.g. translating an HTTP
status to a Code), write an explicit switch in the calling code — do
not work around the missing generic constructor.

**Factories** (one per non-OK Code):

```
NewOpCancelled, NewUnknownError, NewIllegalInput, NewTimeout,
NewNotFound, NewAlreadyExists, NewPermissionDenied, NewUnauthenticated,
NewTooManyRequests, NewFailedPrecondition, NewOpConflict, NewOutOfRange,
NewUnimplemented, NewInternalError, NewUnavailable, NewIllegalState,
NewAuthorizationExpired, NewIllegalArg
```

`NewResourceExhausted` and `NewOpAborted` are deprecated aliases for
`NewTooManyRequests` and `NewOpConflict` — do **not** use them in new
code.

### 2.2 `*apperror.RemoteError`

For server-responded remote failures. Fields:

- `Canonical *AppError` — **must be non-nil**. The normalized view of
  this failure in our taxonomy. Set by the driven adapter at the call
  boundary. `Canonical` is **NOT** a cause; it's a parallel view of the
  same error. Access it explicitly: `r.Canonical.Code()`.
- `Service`, `Operation` — logical service name and logical operation
  name (e.g. `"user-service"`, `"GetUser"`). Operation is a **logical**
  name, not an HTTP method+path.
- `Request *Request` — captured outbound request (optional, often nil
  to avoid logging sensitive bodies).
- `Response *Response` — **must be non-nil**. Captured response.
- `BodyCode`, `BodyMessage` — the remote service's app-level error
  signals parsed from `Response.Body`. Useful as low-cardinality
  observability aggregation keys.
- `RetryAfter time.Duration` — normalized retry hint, if the remote
  provided one.

Methods:
- `StatusCode()` — protocol status code (from `Response.StatusCode`).
- `Event()` — `Service + "." + Operation`.

**RemoteError is a leaf error**: no `Unwrap`, no `Cause` field. The
Canonical view is intentionally not on the `errors.Is`/`errors.As`
chain — consumers access it directly via `r.Canonical`.

`RemoteError` is **not** a subtype of `*AppError`. `errors.As(err, &appErr)`
will NOT recover the Canonical view from a RemoteError. Use
`errors.As(err, &remoteErr)` and then read `remoteErr.Canonical`.

### 2.3 Three "codes" on a RemoteError

```
r.Canonical.Code()  // canonical: our taxonomy. Use for retry / circuit
                    //            breaker / log aggregation.
r.StatusCode()      // protocol: HTTP/RPC status code. Use for retry
                    //           decisions that depend on transport class.
r.BodyCode          // remote app: the remote service's own code string.
                    //             Forensics / runbook reference / a
                    //             low-cardinality observability label.
```

Pivot tip: when logging, write all three as separate structured fields
so dashboards can aggregate by any one of them.

### 2.4 Optional: numeric Case identifiers

If your app's case identifiers need to be numeric (e.g. for stable API
error codes like `"1_3_1042"`), use `apperror/numcase`:

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

For most apps, descriptive string cases via `apperror.NewStrCase("...")`
are simpler and preferred.

---

## 3. Per-layer responsibilities

The architecture has four error-relevant layers. Each has a **distinct**
set of rules. **Always identify which layer you're writing in before
choosing how to handle errors.**

### 3.1 Domain layer

**What it produces**: pure domain errors using `AppError`. The domain
layer must not know about HTTP, RPC, databases, or anything technical.

**Typical codes used**:
- `CodeNotFound` — entity does not exist
- `CodeAlreadyExists` — entity already present
- `CodeFailedPrecondition` — business rule not satisfied
- `CodeOutOfRange` — value outside allowed range
- `CodeIllegalState` — invariant broken; data corruption
- `CodeIllegalArg` — programmer error: invalid args passed within our code

**Patterns**:

```go
// Domain method
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

**Forbidden in domain**:
- Creating `RemoteError` (the domain doesn't call remote services).
- Codes that imply transport/protocol concerns: `CodeOpCancelled`,
  `CodeTimeout`, `CodeUnavailable`, `CodeTooManyRequests`,
  `CodeAuthorizationExpired` etc. — those are technical, not domain.

### 3.2 Application layer

**What it produces**: usually propagates errors from domain and driven
adapters; sometimes constructs its own AppError for use-case-level
preconditions.

**Typical patterns**:

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
        // Domain returned its own AppError; add use-case context.
        if appErr := (*apperror.AppError)(nil); errors.As(err, &appErr) {
            appErr.AddNote("during user.signup")
        }
        return err
    }
    if err := s.users.Insert(ctx, user); err != nil {
        return err   // pass through; users.Insert is a driven-adapter call
    }
    return nil
}
```

**Add use-case context with `AddNote`** — but only when adding context
to the SAME error, not when creating a new error event. See §4.3.

**Forbidden in application**:
- Calling external HTTP/RPC directly (that belongs in driven adapters).
- Hand-rolling translation of HTTP/RPC statuses to our `Code` — that
  also belongs in driven adapters.

### 3.3 Driven adapters (external service clients)

**This is where `RemoteError` is constructed** — and where the
translation between the remote service's error shape and our standardized
taxonomy lives. Every driven adapter for a remote service owns its own
translation function.

**Pattern** for an HTTP-based driven adapter:

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
    _ = json.Unmarshal(body, &envelope) // best-effort parse; missing fields are OK

    canonical := translateUserServiceError(resp.StatusCode, envelope.Code)
    return nil, &apperror.RemoteError{
        Canonical:     canonical,
        Service:       "user-service",
        Operation:     "GetUser",
        Response:      &apperror.Response{StatusCode: resp.StatusCode, Body: body},
        BodyCode:      envelope.Code,
        BodyMessage:   envelope.Message,
        RetryAfter:    parseRetryAfter(resp.Header.Get("Retry-After")),
    }
}

// Each driven adapter owns its own translation.
func translateUserServiceError(status int, bodyCode string) *apperror.AppError {
    // The event slot is required by every factory but ignored by
    // RemoteError.Event() (which reads Service.Operation directly), so a
    // service-scoped placeholder is enough here. Body code wins when
    // present; status is the fallback.
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

**Translation rules**:
- The canonical Code is what upper layers will branch on. Pick it so
  cross-cutting middleware behaves correctly (e.g. retry on
  `CodeUnavailable`, `CodeTimeout`, `CodeTooManyRequests`).
- Do NOT set `WithCause` on `Canonical` — RemoteError is a leaf error.
- Do NOT set `WithDetails` on `Canonical` — remote-side structured info
  belongs in `BodyCode` / `BodyMessage` / `Response.Body`.
- The Canonical's `event` (required by every factory) is ignored at the
  top level — `RemoteError.Event()` is `Service + "." + Operation` and
  never reads the embedded value. Don't optimize for it; pass a
  service-scoped placeholder like `"user-service.translate"`.

**For a service-specific structured envelope** (e.g. Stripe-shaped
errors), define a wrapper type that embeds `*RemoteError`:

```go
type StripeError struct {
    *apperror.RemoteError
    DeclineCode    string
    DeclineMessage string
}
```

Consumers do `errors.As(err, &stripeErr)` when they need Stripe-specific
fields.

### 3.4 Interfaces (driving adapters)

**What it does**: catches errors from Application, translates them into
the wire format of the inbound protocol (HTTP response, gRPC status,
queue NACK, etc.), and logs.

**HTTP-handler pattern**:

```go
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
    user, err := h.svc.GetUser(r.Context(), chi.URLParam(r, "id"))
    if err != nil {
        renderError(w, r, err)
        return
    }
    renderJSON(w, http.StatusOK, user)
}

func renderError(w http.ResponseWriter, r *http.Request, err error) {
    // Step 1 — find the *AppError (works for plain AppError and for
    // RemoteError via r.Canonical, see step 2 below).
    var appErr *apperror.AppError
    var remoteErr *apperror.RemoteError

    switch {
    case errors.As(err, &remoteErr):
        appErr = remoteErr.Canonical
    case errors.As(err, &appErr):
        // appErr is set
    default:
        appErr = apperror.NewInternalError("unhandled error", apperror.WithCause(err))
    }

    // Step 2 — map Code → HTTP status.
    httpStatus, _ := apperror.HTTPStatusFor(appErr.Code())
    if httpStatus == 0 {
        httpStatus = apperror.StatusInternalServerError
    }

    // Step 3 — log structured fields.
    logError(r.Context(), err, appErr, remoteErr)

    // Step 4 — render a sanitized response. Do NOT leak Cause / internal
    // messages directly to clients on 5xx.
    body := errorResponseBody{
        Code:    appErr.Code().Name(),
        Message: appErr.Message(),
    }
    if c := appErr.Case(); c != nil {
        body.Case = c.Identifier()
    }
    renderJSON(w, int(httpStatus), body)
}
```

**Sanitization**: never put internal stack traces, raw transport errors,
or sensitive Request/Response bodies into the response sent to the
client. Log them server-side; expose only `Code`, `Case`, and a
client-safe `Message`.

---

## 4. Common patterns

### 4.1 Adding context as an error propagates up

Use `AddNote` when the SAME error is propagated up and you want to add
upstream context to its message (no new chain layer):

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

The error remains the same `*AppError`; only its Message is prepended
with `" -> "` separated context.

### 4.2 Wrapping a non-AppError

When you catch a `error` that is NOT an AppError (e.g. from a third-party
library, stdlib, or a generic transport call):

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

`WithCause` preserves the original via `errors.Is`/`errors.As`.

### 4.3 When to use AddNote vs `fmt.Errorf %w`

| Goal | Tool |
|---|---|
| Same error, more context, keep `*AppError` type | `appErr.AddNote("...")` |
| New error event, different semantics, grow chain | `fmt.Errorf("ctx: %w", err)` (then consumers `errors.As` to find the inner) |
| Wrap a non-AppError into an AppError | A factory + `WithCause(err)` |

### 4.4 Retry policy

Cross-cutting retry logic should branch on Canonical Code, falling back
to RemoteError's protocol/body signals:

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

Note the **two `errors.As` calls**. Because `RemoteError`'s Canonical is
not on the `errors.As` chain, you cannot consolidate this into a single
"find any AppError" lookup — that's intentional. Always check for
`*RemoteError` first.

### 4.5 Structured logging

Log the following fields whenever you log an error. Field names are a
recommended convention:

```
level=error
event=<appErr.Event() or remoteErr.Event()>
code=<appErr.Code().Name()>
case=<appErr.Case().Identifier() if non-nil>
message=<appErr.Message()>
// For RemoteError, additionally:
service=<remoteErr.Service>
operation=<remoteErr.Operation>
status=<remoteErr.StatusCode()>
body_code=<remoteErr.BodyCode>
body_message=<remoteErr.BodyMessage>
retry_after=<remoteErr.RetryAfter>
```

`event` makes a great log primary key; `code` / `case` / `body_code`
make good aggregation labels. **Do not** log raw `Response.Body` or
captured `Request.Body` unconditionally — they may contain sensitive
data; redact at the logging boundary.

### 4.6 When to define a specific Case

**Default: don't.** The `Code` alone is enough most of the time, and an
empty Case keeps log/metric cardinality low and the API surface small.

**Add a Case only when product design or a caller needs to distinguish
this specific situation from others sharing the same Code** — to present
a friendlier user-facing prompt or to trigger a different follow-up flow.
If nothing branches on it, don't mint it.

**Example — earns its keep.** During Account signup with
`CodeAlreadyExists`, the UI wants to swap the generic "already registered"
message for a CTA into the password-recovery flow when the conflict is on
email or phone:

```go
// In domain or application layer
if existing, _ := repo.FindByEmail(ctx, email); existing != nil {
    return apperror.NewAlreadyExists(
        "account.create",
        apperror.WithMessage("an account with this email already exists"),
        apperror.WithCase(apperror.NewStrCase("account_credential_taken")),
    )
}
```

Without the Case, the client has only `CodeAlreadyExists` — which could
equally mean a duplicate username, a duplicate display name, or a
duplicate idempotency key — and would have to fall back to parsing
`Message`, which is brittle.

**Anti-example — pure noise.** Don't mint a Case just to mirror an
internal failure mode that no caller acts on:

```go
// Bad — adds cardinality with no consumer.
apperror.NewIllegalInput(
    "user.signup",
    apperror.WithMessage("email must contain @"),
    apperror.WithCase(apperror.NewStrCase("email_missing_at")),
)
```

`Message + Code` already carries everything useful for an input
validation error like this. A Case here only inflates dashboards and
gives future readers the false impression that someone, somewhere
branches on it.

**The threshold question, in order**: before adding `WithCase(...)`, ask
— (1) Is there a concrete consumer (UI screen, partner contract, runbook
step) that will behave differently for this Case vs. a sibling Case under
the same Code? (2) Would `Message` alone be too brittle for that
consumer? If both yes → define the Case. Otherwise, skip it.

---

## 5. Decision tree: which type / code do I use?

```
Did the error happen inside our app's logic (no remote call involved)?
├─ YES → AppError, factory chosen by the failure semantics
│         (see "Typical codes used" in §3.1 + §3.2)
└─ NO → We were calling a remote service.
        ├─ Did the server send back a response?
        │   ├─ NO  → AppError. Use NewUnavailable for refused/reset/DNS,
        │   │        NewTimeout for context deadline; WithCause(transportErr).
        │   └─ YES → RemoteError. Driven adapter constructs it with
        │            a Canonical *AppError + Response + BodyCode/BodyMessage.
        └─ Status / body indicates success?
            └─ Return the parsed result; not an error.
```

For choosing the Canonical Code when constructing a RemoteError, the
driven adapter's translation function decides. Default mapping for HTTP
statuses can use `apperror.OpCodeFor(statusCode)` but most adapters will
need service-specific overrides for in-body error codes.

---

## 6. Anti-patterns — DO NOT

1. **Do NOT construct an AppError with `CodeOK`.** No factory exists for
   it. CodeOK is preserved only for the Code↔HTTP-status mapping.

2. **Do NOT include the failure mode in `event`.** Event names the
   operation: `"user.signup"`. The failure mode belongs in `Code` /
   `Case`. Writing `event="user.signup.email_invalid"` collapses two
   independent observability dimensions into one and breaks
   `event × code` pivots.

3. **Do NOT use `\n` in error messages.** Log aggregators split on
   newlines and break a single error into multiple log events. Keep
   messages single-line; the `" -> "` separator from `AddNote` is the
   right tool for multi-layer context.

4. **Do NOT create a RemoteError for a transport-layer failure.**
   No response was received → AppError. The presence of a Response is
   what makes a failure a RemoteError.

5. **Do NOT set `WithCause` or `WithDetails` on the `Canonical` of a
   RemoteError.** RemoteError is a leaf with its own typed fields for
   those concerns; setting them on Canonical creates ambiguity:
   - Cause: RemoteError has no cause (it's the server's response itself).
   - Details: use `BodyCode` / `BodyMessage` / `Response.Body` instead.

   The Canonical's `event` (required by every factory) is ignored at
   the top level — `RemoteError.Event()` reads `Service.Operation` and
   never the embedded value. Don't optimize for it; pass a
   service-scoped placeholder like `"user-service.translate"`.

6. **Do NOT expect `errors.As(err, &appErr)` to recover the Canonical
   view from a RemoteError.** Canonical is a parallel view, not in the
   `errors.Is`/`errors.As` chain. Use `errors.As(err, &remoteErr)`,
   then read `remoteErr.Canonical` explicitly.

7. **Do NOT mint your own `Code` values** (e.g. `apperror.Code(100)`).
   The package's taxonomy is closed; if you find your error doesn't fit
   any existing Code, that's a sign you should reconsider the semantic
   (likely the closest fit + a specific Case is what you want).

8. **Do NOT leak raw upstream / cause details to API clients.** Sanitize
   at the Interfaces layer: expose `Code.Name()`, `Case.Identifier()`,
   `Message()` — but never raw stack traces, transport errors, or
   third-party response bodies unless the user explicitly opted in (e.g.
   internal debug endpoint).

9. **Do NOT add methods to AppError for forwarding to remote-specific
   data.** AppError is the domain-error type; remote-specific
   information belongs on RemoteError. Don't blur this boundary.

10. **Do NOT translate remote errors in the Application or Domain
    layers.** Translation lives in driven adapters at the boundary.
    Upper layers receive `*AppError` (or `*RemoteError` if they have a
    legitimate need to inspect remote specifics for retry/observability).

---

## 7. Quick reference cheat-sheet

```
DOMAIN LAYER         → AppError only; codes: NotFound, AlreadyExists,
                        FailedPrecondition, OutOfRange, IllegalState,
                        IllegalArg
APPLICATION LAYER    → AppError (propagates or constructs); AddNote for
                        upstream context; no protocol translation
DRIVEN ADAPTER       → AppError for transport failures
                       RemoteError for server-responded failures
                       Owns translation: remote signals → Canonical Code
INTERFACES LAYER     → errors.As to *RemoteError / *AppError, map to wire
                       format, log structured fields, sanitize response

Construction         → per-Code factory(event, opts...) +
                       WithMessage / WithCase / WithCause / WithDetails
Propagation          → AddNote for same error; fmt.Errorf %w for new layer
Cross-cutting        → branch on appErr.Code() / remoteErr.Canonical.Code()
                       NOT on errors.As(err, &appErr) when going through
                       a RemoteError (Canonical is parallel, not on chain)
```

When in doubt, re-read §1 (the three categories) and §5 (the decision
tree). Most mistakes start there.
