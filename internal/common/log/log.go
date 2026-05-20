// Package log builds and exposes the application's structured logger.
//
// Layout:
//   - Init (this file) configures the process-wide *slog.Logger (installed
//     as slog.Default) and returns the matching *slog.HandlerOptions so
//     framework adapters (e.g. the Hertz hlog → slog bridge) share one
//     source of truth for level / format.
//   - The DebugAttrs/InfoAttrs/WarnAttrs/ErrorAttrs wrappers (this file)
//     are the call-site API. They take ctx + a required event + an
//     optional msg + ...slog.Attr.
//   - ErrAttrs (err_attrs.go) turns an *AppError / *RemoteError into the
//     structured fields prescribed by error_handling_guide.md §4.5;
//     ErrGroup wraps those fields in a nested "err" group attr.
//
// Two interchangeable strategies carry request-scoped attrs (request_id,
// trace_id, …) to every log line. They are deliberately kept side by side
// as a reference; pick one per app:
//
//   - Approach A — "logger in context" (ctx_logger.go): store the
//     request-scoped *slog.Logger in ctx; read it back with FromCtx.
//     This is the strategy currently wired in request_logger.go.
//   - Approach B — "values in context + enriching handler" (ctx_handler.go):
//     store request-scoped VALUES in ctx; a ctxHandler (installed by Init)
//     injects them per record. This is the slog-idiomatic path; it is
//     installed and ready, one line away from being active.
//
// The wrappers below serve BOTH unchanged — see logAttrs.
//
// Importers use the package as `applog`:
//
//	import applog "go-app-template/internal/common/log"
package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Init configures slog.Default with a JSON or text handler writing to w,
// at the given level. It returns the constructed logger and the
// *slog.HandlerOptions used, so callers can hand both to the hlog bridge
// (NewHlogBridge) and keep one source of truth for level/format across the
// app's logs and Hertz's framework logs.
//
// format is "json" by default; pass "text" for human-readable local dev.
// level accepts "debug" / "info" / "warn" / "error" (case-insensitive).
// A nil writer falls back to os.Stdout.
func Init(w io.Writer, format, level string) (*slog.Logger, *slog.HandlerOptions) {
	if w == nil {
		w = os.Stdout
	}
	// Level is a *slog.LevelVar (not a static Level) so it can be retuned at
	// runtime — both by an operator path and by the hlog bridge's SetLevel,
	// which reuses this same var so the whole process shares one level knob.
	lvl := new(slog.LevelVar)
	lvl.Set(parseLevel(level))
	opts := &slog.HandlerOptions{Level: lvl}

	var base slog.Handler
	if strings.EqualFold(format, "text") {
		base = slog.NewTextHandler(w, opts)
	} else {
		base = slog.NewJSONHandler(w, opts)
	}
	// Approach B: wrap the base handler so every record is enriched with
	// request-scoped values pulled from ctx (see ctx_handler.go). It is a
	// no-op when those values are absent, so installing it unconditionally
	// costs nothing under Approach A — the two never double-count request_id.
	l := slog.New(newCtxHandler(base))
	slog.SetDefault(l)
	return l, opts
}

// DebugAttrs / InfoAttrs / WarnAttrs / ErrorAttrs are level-bound
// wrappers around slog.Logger.LogAttrs that:
//
//   - take event as a REQUIRED positional parameter — the operation name
//     in "{namespace}.{operation}" form (per
//     error_handling_guide.md §4.5: "event makes a great log primary
//     key"). Passing "" is legal but defeats the purpose; an empty
//     event lands in the log as `event=""` which is intentionally
//     easy to spot in dashboards.
//   - take msg as a free-form description that MAY be empty — common
//     for failure logs where ErrAttrs already supplies the `message`
//     field from the underlying AppError, and `msg` would just
//     duplicate it.
//   - resolve the logger via FromCtx, eliminating the double-ctx
//     awkwardness of FromCtx(ctx).LogAttrs(ctx, ...). This one call works
//     for BOTH strategies (see logAttrs).
//   - take ...slog.Attr (not ...any), so the type-safety / zero-alloc
//     story of LogAttrs is preserved.
//
// All four route through logAttrs, which prepends event so it lands
// near the top of the JSON record. That extra Attr costs one slice
// allocation per call — net-equal to writing slog.String("event", ...)
// at every call site by hand, which is what this API replaces.
func DebugAttrs(ctx context.Context, event, msg string, attrs ...slog.Attr) {
	logAttrs(ctx, slog.LevelDebug, event, msg, attrs...)
}

func InfoAttrs(ctx context.Context, event, msg string, attrs ...slog.Attr) {
	logAttrs(ctx, slog.LevelInfo, event, msg, attrs...)
}

func WarnAttrs(ctx context.Context, event, msg string, attrs ...slog.Attr) {
	logAttrs(ctx, slog.LevelWarn, event, msg, attrs...)
}

func ErrorAttrs(ctx context.Context, event, msg string, attrs ...slog.Attr) {
	logAttrs(ctx, slog.LevelError, event, msg, attrs...)
}

func logAttrs(ctx context.Context, level slog.Level, event, msg string, attrs ...slog.Attr) {
	all := make([]slog.Attr, 0, len(attrs)+1)
	all = append(all, slog.String("event", event))
	all = append(all, attrs...)
	// FromCtx serves both strategies, so call sites never branch on which
	// is active:
	//   - Approach A: returns the request logger bound in ctx (request_id
	//     already attached via .With).
	//   - Approach B: finds nothing bound, falls back to slog.Default(),
	//     whose ctxHandler injects request_id from the ctx value.
	FromCtx(ctx).LogAttrs(ctx, level, msg, all...)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
