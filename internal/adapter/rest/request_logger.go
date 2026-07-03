package rest

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"

	applog "go-app-template/internal/common/log"
)

const (
	requestIDHeader = "X-Request-Id"

	// requestEvent names the access-log operation. Pure operation name;
	// status / latency / outcome live in attrs, never in the event string.
	requestEvent = "rest.request"
)

// RequestLogger is the inbound middleware that owns per-request logging
// observability. It does three things:
//
//  1. Establishes a request_id — honors a caller-supplied
//     X-Request-Id header (so distributed traces stitch) or mints a
//     fresh UUID; echoes the chosen value back in the response header.
//  2. Derives a *slog.Logger pre-bound with request_id and stashes it
//     in ctx so downstream handlers and services log under the same
//     correlation key via applog.FromCtx(ctx).
//  3. Emits exactly one "request handled" access-log line after the
//     handler chain returns, with method, path, status, and latency.
//
// /health/* probes are skipped from the access log: in a Kubernetes
// deployment they fire every few seconds per pod, and the resulting
// volume drowns the signal without adding any operational value.
//
// Register on the engine before any route is registered:
//
//	h := server.Default(...)
//	h.Use(rest.RequestLogger())
//	// ...handlers...
func RequestLogger() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()
		path := string(c.Path())

		reqID := string(c.GetHeader(requestIDHeader))
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Header(requestIDHeader, reqID)

		// === Approach A (active): store the request logger in ctx. ===
		// Make ctx the single source of truth for the request logger: one
		// IntoCtx serves both the downstream handler chain (via c.Next) and
		// this middleware's own access-log line below (applog.InfoAttrs reads
		// it back through FromCtx). Reassigning ctx — instead of handing a
		// throwaway child to c.Next — keeps the access log on the same
		// request_id rather than falling back to slog.Default().
		ctx = applog.IntoCtx(ctx, slog.Default().With(slog.String("request_id", reqID)))

		// === Approach B (alternative): store the request_id VALUE in ctx
		// and let applog's ctxHandler inject it per record. To switch, swap
		// the IntoCtx line above for:
		//     ctx = applog.WithRequestID(ctx, reqID)
		// Nothing else changes — the InfoAttrs call below and every
		// downstream applog.*Attrs(ctx, …) behave identically under both. ===
		c.Next(ctx)

		if strings.HasPrefix(path, "/health/") {
			return
		}
		applog.InfoAttrs(ctx, requestEvent, "request handled",
			slog.String("method", string(c.Method())),
			slog.String("path", path),
			slog.Int("status", c.Response.StatusCode()),
			slog.Duration("latency", time.Since(start)),
		)
	}
}
