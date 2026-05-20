package log

import (
	"context"
	"log/slog"
)

// ===========================================================================
// Approach B — "values in context + an enriching handler"
//
// The middleware stores request-scoped VALUES (request_id, …) in ctx with
// WithRequestID. The logger is NOT stored in ctx — it stays explicit
// (slog.Default() or an injected *slog.Logger). ctxHandler, installed as the
// default handler by Init, reads those values out of ctx at Handle time and
// adds them as attrs to every record.
//
//	Pro:  request-scoped data lives in ctx (its intended use); the logger is
//	      an explicit dependency; ANY logger whose handler is ctxHandler gets
//	      enriched, so even a plain slog.InfoContext(ctx, …) carries
//	      request_id; no per-request logger allocation.
//	Con:  one extra handler type to maintain; the set of injected fields is
//	      fixed here in Handle, not chosen by the caller.
//
// This is the strategy slog is designed around — the ...Context log methods
// exist precisely so handlers can read ctx. It is installed and ready; to
// make it ACTIVE, switch request_logger.go from IntoCtx to WithRequestID
// (one line, marked there). Its companion is Approach A in ctx_logger.go.
// ===========================================================================

type requestIDKey struct{}

// WithRequestID returns a child ctx carrying reqID. ctxHandler reads it back
// and adds a "request_id" attr to every record logged with this ctx.
func WithRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, reqID)
}

// RequestIDFromCtx returns the request id carried by ctx, or "" if none.
func RequestIDFromCtx(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// ctxHandler decorates a base slog.Handler, injecting request-scoped attrs
// pulled from ctx into each record. To surface a new field on every line,
// read another ctx value in Handle — that single edit is all it takes.
type ctxHandler struct {
	slog.Handler // embedded: Enabled (and any future methods) delegate through
}

func newCtxHandler(base slog.Handler) ctxHandler {
	return ctxHandler{Handler: base}
}

// Handle is where enrichment happens: pull request-scoped values from ctx
// and attach them before delegating to the base handler.
func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFromCtx(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs / WithGroup must re-wrap so the decorator survives any
// logger.With(...) / .WithGroup(...) — otherwise those return the bare base
// handler and silently drop ctx enrichment. (Caveat: after a handler-level
// WithGroup, injected attrs nest under that group; the app opens groups
// per-call, not on the handler, so this doesn't bite here.)
func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{Handler: h.Handler.WithGroup(name)}
}
