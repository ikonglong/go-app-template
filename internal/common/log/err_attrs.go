package log

import (
	"errors"
	"log/slog"

	"github.com/ikonglong/go-apperror"
)

// ErrAttrs returns the structured-log fields prescribed by
// error_handling_guide.md §4.5 for the given error:
//
//	code, case, message, cause
//
// plus, when the chain contains a *RemoteError, the remote-specific
// fields:
//
//	service, operation, status, body_code, body_message, retry_after
//
// cause is the AppError's underlying Cause() — the wrapped root error
// (e.g. a db driver or transport failure) — included so the server-side
// log keeps the root cause for diagnosis. It is intentionally log-only:
// the inbound boundary's errorResp exposes just code/case/message, never
// cause (guide §3.4 — "log them server-side; expose only Code/Case/
// Message"). It is omitted when the error has no cause.
//
// Note: event is intentionally NOT emitted here. The caller (via the
// InfoAttrs/WarnAttrs/ErrorAttrs/DebugAttrs wrappers) supplies event as
// the operation it was performing at log time — which is what dashboards
// pivot on. If err was born inside a sub-operation with a different
// Event(), preserving both would put two "event" keys in the JSON and
// muddy the primary aggregation key; the caller's event wins by design.
//
// Errors that are neither *AppError nor wrap one collapse to a single
// "error" attr carrying err.Error(); that path should be rare — the
// inbound boundary wraps unknown errors as Internal before logging.
//
// ErrAttrs returns the fields flat. To render them nested under an "err"
// object instead, use ErrGroup, which wraps this output in a group attr.
//
// Typical use (flat):
//
//	applog.ErrorAttrs(ctx, appErr.Event(), "request failed", applog.ErrAttrs(err)...)
func ErrAttrs(err error) []slog.Attr {
	if err == nil {
		return nil
	}
	attrs := make([]slog.Attr, 0, 8)

	var (
		re *apperror.RemoteError
		ae *apperror.AppError
	)

	// When a RemoteError is the cause, surface its remote-specific fields.
	// In go-apperror v0.2.0 the canonical *AppError wraps the RemoteError as
	// its cause, so both are on the same errors.As chain: errors.As(&re)
	// reaches the wrapped RemoteError and errors.As(&ae) below reaches the
	// canonical AppError that carries code/case/message.
	if errors.As(err, &re) {
		attrs = append(attrs,
			slog.String("service", re.Service),
			slog.String("operation", re.Operation),
		)
		if re.Response != nil {
			attrs = append(attrs, slog.Int("status", re.Response.StatusCode))
		}
		if re.BodyCode != "" {
			attrs = append(attrs, slog.String("body_code", re.BodyCode))
		}
		if re.BodyMessage != "" {
			attrs = append(attrs, slog.String("body_message", re.BodyMessage))
		}
		if re.RetryAfter > 0 {
			attrs = append(attrs, slog.Duration("retry_after", re.RetryAfter))
		}
	}

	// The canonical AppError carries the normalized code/case/message. It is
	// the outer layer when a RemoteError is wrapped, and the whole error
	// otherwise. A chain that is neither an AppError nor wraps one collapses
	// to a single "error" attr (rare — the inbound boundary wraps unknown
	// errors as Internal before logging). Append rather than replace so any
	// remote fields already gathered above survive an unwrapped RemoteError
	// (which carries no canonical AppError) instead of being dropped.
	if !errors.As(err, &ae) {
		return append(attrs, slog.String("error", err.Error()))
	}

	attrs = append(attrs,
		slog.String("code", ae.Code().Name()),
		slog.String("message", ae.Message()),
	)
	if c := ae.Case(); c != nil {
		attrs = append(attrs, slog.String("case", c.Identifier()))
	}
	if cause := ae.Cause(); cause != nil {
		attrs = append(attrs, slog.String("cause", cause.Error()))
	}
	return attrs
}

// ErrGroup wraps the fields ErrAttrs produces into a single "err" group
// attr, so they render as a nested object (JSON: "err":{"code":…,
// "message":…}; text: err.code=… err.message=…) instead of sitting flat
// alongside the record's other keys. Use this at a boundary whose log
// line also carries non-error groups (e.g. a "req" group) and you want
// error fields namespaced together rather than intermixed.
//
// For unexpected (500-class) errors it also nests a "stack" field
// (err.stack) carrying the captured call stack(s) — see StackAttrs for the
// gating and shape. Routine, expected errors add no stack.
//
// A nil err yields an empty group, which slog omits from the output.
func ErrGroup(err error) slog.Attr {
	attrs := append(ErrAttrs(err), StackAttrs(err)...)
	return slog.Attr{Key: "err", Value: slog.GroupValue(attrs...)}
}
