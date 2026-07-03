# Error Handling Guide — Go + `go-apperror`

This guide is the **Go + `go-apperror` realization** of the universal **Error
Handling** framework (the project's standing standard, *企业应用错误处理指南*).
It applies to a Go application that adopts ports-and-adapters architecture and
uses `github.com/ikonglong/go-apperror` as its error model.

This guide assumes you have read the base framework. It does **not** restate that
reasoning; it gives the concrete Go realization: the types, the per-Code
factories, the idioms, and runnable code. Each section points back to the
corresponding base section via `(base → ...)` markers.

The base defines the unified error model, the two responsibility-assignment
contexts (判责语境), the handling mechanism (translate → wrap → expose), the
business-vs-technical classification, and the retry / logging / sub-case
guidance. The rules here are normative — when this guide and a local convention
disagree, this guide wins; when this guide and the base disagree, the **base**
wins. Flag either conflict to the user.

---

## 1. The two error types

### 1.1 `*apperror.AppError`

**Construct only via per-Code factories**, all sharing one signature:

```go
func NewXxx(event string, opts ...Option) *AppError
```

- **`event`** (first positional, *required*) — `"{namespace}.{operation}"`
  (e.g. `"user.signup"`). Factories panic on empty event.
- **`WithMessage("...")`** — optional; falls back to `Code.Description()`.
- **`WithCase(...)`**, **`WithCause(...)`**, **`WithDetails(...)`** — the
  remaining optional fields.

```go
apperror.NewIllegalInput("user.signup", apperror.WithMessage("email must contain @"))
apperror.NewNotFound("user.lookup", apperror.WithMessage("user not found"))
apperror.NewInternal("user.lookup",
    apperror.WithMessage("db query failed"), apperror.WithCause(err))
```

There is **no** generic `apperror.New(code, ...)`. For a runtime-determined Code
(rare; e.g. translating an HTTP status), write an explicit switch.

**Factories** (one per non-OK Code):

```
NewCancelled, NewUnknown, NewIllegalInput, NewTimeout,
NewNotFound, NewAlreadyExists, NewPermissionDenied, NewUnauthenticated,
NewTooManyRequests, NewFailedPrecondition, NewConflict, NewOutOfRange,
NewUnimplemented, NewInternal, NewUnavailable, NewIllegalState,
NewUnauthorized, NewIllegalArg
```

Go-accessible fields: `Code()`, `Case()`, `Message()`, `Event()`, `Details()`,
`Cause()` / `Unwrap()`, `StackTrace()`. `StackTrace()` is rendered to logs only
for unexpected (500-class) codes, never to the client.

### 1.2 `*apperror.RemoteError`

`RemoteError` carries its own `Event()`, `Code()`, `Case()`, `Message()`,
`Details()` — the same field set as `AppError`. It adds `ErrResp()` for
protocol-level details of a server-responded failure, and `Cause()` / `Unwrap()`
for the underlying transport error.

**`RemoteErrorResp`** — a DTO parsed from the remote's response (not an error):
`StatusCode()`, `BodyCode`, `BodyMessage`, `RetryAfter`.

**Construct via `NewRemoteXxx` factories** (one per Code, mirroring AppError's
factories), taking `RemoteOption`s:

```go
// Remote responded with an error:
re := apperror.NewRemoteUnavailable("SearchService.search",
    apperror.RemoteWithMessage("search service unavailable"),
    apperror.WithErrResp(&apperror.RemoteErrorResp{
        Response:    &apperror.Response{StatusCode: 503},
        BodyCode:    "DEGRADED",
        BodyMessage: "service in maintenance",
        RetryAfter:  30 * time.Second,
    }),
)

// Transport failure (no response):
re := apperror.NewRemoteUnavailable("SearchService.search",
    apperror.RemoteWithMessage("search service unreachable"),
    apperror.RemoteWithCause(connErr),
)
```

**`RemoteOption`s**:

| Option | Purpose |
|---|---|
| `RemoteWithMessage(msg)` | Set the message; falls back to `Code.Description()` |
| `RemoteWithCause(err)` | Set the cause; enables `errors.Is`/`errors.As` chain |
| `RemoteWithCase(c)` | Attach a `Case` (rare) |
| `RemoteWithDetails(d)` | Attach ad-hoc structured details |
| `WithErrResp(resp)` | Attach the parsed `RemoteErrorResp` |

At least one of `WithErrResp` or `RemoteWithCause` must be set — the factory
panics otherwise.

**Rules:**

- A Driven adapter **must** wrap remote-call failures as a `*RemoteError`
  (base → *架构约束*). It *may* propagate through a port into Core — the Core
  wraps it into an `AppError`.
- Do **not** construct a `RemoteError` for app-internal failures — those are
  `AppError`s.

---

## 2. Per-layer realization

The base's mechanism is **translate → wrap → expose**. Below is the Go code for
each layer.

### 2.1 Domain layer

Produces errors as `AppError`. Never builds a `RemoteError` — the domain has no
notion of a remote service. Which codes the domain emits follows from the rules
and invariants it enforces, not from a code-to-layer mapping (base →
*Business/Technical Error*).

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

Usually propagates errors from domain and driven adapters; sometimes constructs
its own `AppError` for use-case-level preconditions.

This is where **wrap** happens (base → *如何处理原始远程错误 / 处理步骤*).
When wrapping a `RemoteError`, keep it as the `AppError`'s cause via
`WithCause(re)`:

```go
func (s *CheckoutService) Checkout(ctx context.Context, cartID string) error {
    user, err := s.users.FindByID(ctx, cartID)
    if err != nil {
        var re *apperror.RemoteError
        if errors.As(err, &re) {
            return apperror.NewUnavailable("checkout.charge",
                apperror.WithMessage("user service unavailable during checkout"),
                apperror.WithCause(re),
            )
        }
        return err
    }
    // ...
}
```

### 2.3 Driven adapters (external service clients)

The **translate** site (base → *如何处理原始远程错误 / 处理步骤*). Every
driven adapter translates raw remote failures into a `RemoteError`, preserving
the original error as the `Cause`.

Two sub-cases:

- **Remote returned an error response**: parse the body → `RemoteErrorResp` →
  `NewRemoteXxx` + `WithErrResp`.
- **Network failure or timeout (no response)**: `NewRemoteXxx` +
  `RemoteWithCause` (typically `CodeUnavailable` or `CodeTimeout`).

```go
func (c *UserServiceClient) GetUser(ctx context.Context, id string) (*User, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", c.base+"/users/"+id, nil)
    resp, err := c.http.Do(req)

    // Network / timeout failure: RemoteError + RemoteWithCause
    if err != nil {
        return nil, apperror.NewRemoteUnavailable("UserService.GetUser", apperror.RemoteWithCause(err))
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)

    // Success
    if resp.StatusCode < 400 {
        var u User
        if err := json.Unmarshal(body, &u); err != nil {
            return nil, apperror.NewInternal(
                "UserService.GetUser",
                apperror.WithMessage("decoding user-service response"),
                apperror.WithCause(err),
            )
        }
        return &u, nil
    }

    // Server responded with failure: build RemoteErrorResp → NewRemoteXxx + WithErrResp
    var envelope struct {
        Code    string `json:"code"`
        Message string `json:"message"`
    }
    _ = json.Unmarshal(body, &envelope)

    return nil, translateUserServiceError("UserService.GetUser",
        resp.StatusCode, body, envelope.Code, envelope.Message,
        parseRetryAfter(resp.Header.Get("Retry-After")),
    )
}

func translateUserServiceError(
    event string, statusCode int, body []byte,
    bodyCode, bodyMessage string, retryAfter time.Duration,
) *apperror.RemoteError {
    errResp := &apperror.RemoteErrorResp{
        Response:    &apperror.Response{StatusCode: statusCode, Body: body},
        BodyCode:    bodyCode,
        BodyMessage: bodyMessage,
        RetryAfter:  retryAfter,
    }
    switch bodyCode {
    case "USER_GONE":
        return apperror.NewRemoteNotFound(event, apperror.WithErrResp(errResp))
    case "RATE_LIMITED":
        return apperror.NewRemoteTooManyRequests(event, apperror.WithErrResp(errResp))
    }
    switch {
    case statusCode == 404:
        return apperror.NewRemoteNotFound(event, apperror.WithErrResp(errResp))
    case statusCode == 429:
        return apperror.NewRemoteTooManyRequests(event, apperror.WithErrResp(errResp))
    case statusCode >= 500:
        return apperror.NewRemoteUnavailable(event, apperror.WithErrResp(errResp))
    default:
        return apperror.NewRemoteUnknown(event, apperror.WithErrResp(errResp))
    }
}
```

### 2.4 Interfaces (driving adapters)

The **expose** step (base → *如何处理应用自身产生的错误 / 处理步骤*). Maps
`AppError.Code()` to HTTP status; any non-`AppError` arriving here is wrapped as
`Internal` — the last line of defense. Logs the full error server-side; sanitizes
the response.

```go
func renderError(w http.ResponseWriter, r *http.Request, err error) {
    var appErr *apperror.AppError
    if !errors.As(err, &appErr) {
        appErr = apperror.NewInternal("rest.unhandled", apperror.WithCause(err))
    }

    httpStatus, ok := apperror.HTTPStatusFor(appErr.Code())
    if !ok {
        httpStatus = apperror.StatusInternalServerError
    }

    logError(r.Context(), err, appErr)

    body := errorResponseBody{Code: appErr.Code().Name(), Message: appErr.Message()}
    if c := appErr.Case(); c != nil {
        body.Case = c.Identifier()
    }
    renderJSON(w, int(httpStatus), body)
}
```

**Sanitize**: never expose internal stack traces, raw transport errors, or
sensitive request/response bodies to the client. Log them server-side; expose
only `Code`, `Case`, and a client-safe `Message`.

---

## 3. Idioms

Go realizations of the base's error-propagation and cross-cutting guidance.

### 3.1 Add context as an error propagates up

`AddNote` prepends upstream context to the same error's message via `" -> "`
separator — no new chain layer:

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

### 3.2 Wrap a non-AppError

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

| Goal | Tool |
|---|---|
| Same error, more context, keep `*AppError` type | `appErr.AddNote("...")` |
| New error event, different semantics, grow chain | `fmt.Errorf("ctx: %w", err)` |
| Wrap a non-AppError into an AppError | factory + `WithCause(err)` |

### 3.4 Retry policy

Branch on `Code` — recover it via `errors.As(err, &ae)` for `AppError`, or
`errors.As(err, &re)` then `re.Code()` for `RemoteError`. Follow the base's
retryable taxonomy exactly (base → *Client 侧重试* + *Server 侧重试与恢复*):
`CodeUnavailable` is retryable; `CodeTimeout`, `CodeInternal`, `CodeUnknown`,
`CodeConflict`, and all Client-fault codes are not.

### 3.5 Structured logging

In this project the fields and stack come from `applog.ErrAttrs` /
`applog.ErrGroup`, and the `event` from the level wrappers
`applog.ErrorAttrs(ctx, event, msg, …)`. The field set:

```
event        = appErr.Event()
code         = appErr.Code().Name()
case         = appErr.Case().Identifier()   // if non-nil
message      = appErr.Message()
cause        = appErr.Cause().Error()       // if non-nil — SERVER-SIDE ONLY
// When the cause is a RemoteError, additionally:
service      = (from RemoteError fields)
status       = remoteErr.ErrResp().StatusCode()
body_code    = remoteErr.ErrResp().BodyCode
// For unexpected (500-class) codes, additionally:
stack        = captured call stack(s)       // SERVER-SIDE ONLY
```

`stack` is a `[][]string` with no embedded newlines, rendered only for
`Internal`, `Unknown`, `IllegalState`. **Do not** log raw response bodies
unconditionally — redact at the boundary.

### 3.6 When to define a specific Case

**Default: don't.** Add a `Case` only when a concrete consumer will branch on it.

**Justified** — signup `CodeAlreadyExists` where the UI swaps the generic message
for a password-recovery CTA:

```go
if existing, _ := repo.FindByEmail(ctx, email); existing != nil {
    return apperror.NewAlreadyExists(
        "account.create",
        apperror.WithMessage("an account with this email already exists"),
        apperror.WithCase(apperror.NewStrCase("account_credential_taken")),
    )
}
```

**Unjustified** — a Case mirroring an internal failure mode no caller acts on:

```go
// Bad — cardinality with no consumer.
apperror.NewIllegalInput("user.signup",
    apperror.WithMessage("email must contain @"),
    apperror.WithCase(apperror.NewStrCase("email_missing_at")))
```

---

## 4. Anti-patterns — DO NOT

The base flags the universal traps. These are the **`go-apperror`-specific**
ones:

1. **Do NOT construct an AppError with `CodeOK`.** No factory exists; CodeOK is
   only for the Code↔HTTP-status mapping.
2. **Do NOT mint your own `Code` values** (e.g. `apperror.Code(100)`). The
   taxonomy is closed; if nothing fits, use the closest Code + a specific Case.
3. **Do NOT return an `AppError` from a Driven adapter for a remote-call
   failure.** The architecture constraint (base → *架构约束*) requires
   `RemoteError` from driven adapters for remote failures.
4. **Do NOT let a `RemoteError` reach the Interfaces layer unwrapped.**
   Core must wrap it into an `AppError` (base → *如何处理原始远程错误 / 处理步骤*).
5. **Do NOT encode the failure mode in `event`.** The event names the operation
   (e.g. `"user.signup"`); the Code and Message describe the failure. A
   `"user.signup.email_invalid"` event duplicates the Code's job and explodes
   event cardinality.
6. **Do NOT embed newlines in messages.** Use `AddNote`'s `" -> "` separator
   to prepend context as the error propagates up.

When in doubt, re-read the base's handling mechanism and 判责 guidance.
