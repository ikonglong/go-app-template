package log

import (
	"context"
	"log/slog"
)

// ===========================================================================
// Approach A — "logger in context"
//
// The middleware derives a request-scoped *slog.Logger (pre-bound with
// request_id via .With) and stores the LOGGER ITSELF in ctx. Downstream code
// pulls it back out with FromCtx and logs through it.
//
//	Pro:  zero per-component wiring — anything holding ctx can log with the
//	      request's attrs for free.
//	Con:  stores a dependency in ctx, which the Go context guidance leans
//	      against ("request-scoped data, not optional parameters"); the
//	      logging dependency stays invisible in function signatures.
//
// This is the strategy currently wired in request_logger.go. Its companion
// is Approach B in ctx_handler.go — pick one per app.
// ===========================================================================

type ctxKey struct{}

// FromCtx returns the logger bound to ctx by IntoCtx, falling back to
// slog.Default when none is present. The result is always non-nil so callers
// can chain (.LogAttrs / .With / etc.) without a guard.
//
// That fallback is also what lets the Xxx Attrs wrappers serve Approach B
// unchanged: under B nothing is bound, so FromCtx returns slog.Default(),
// whose ctxHandler enriches the record from the ctx value.
func FromCtx(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// IntoCtx returns a child context with l bound under the package's private
// key. Inbound middleware calls this once per request after deriving a
// logger with request-scoped attrs (request_id, etc.); downstream code reads
// the logger back via FromCtx.
func IntoCtx(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}
