package log

import (
	"errors"
	"log/slog"

	"github.com/ikonglong/go-apperror"
)

// ErrAttrs returns the structured-log fields for the given error:
//
//	code, case, message, cause
//
// plus, when the chain contains a *RemoteError with ErrResp(), the
// remote-specific fields:
//
//	status, body_code, body_message, retry_after
//
// cause is the underlying Cause() — the wrapped root error (e.g. a db driver
// or transport failure) — included so the server-side log keeps the root cause
// for diagnosis. It is intentionally log-only: the inbound boundary's
// errorResp exposes just code/case/message, never cause. It is
// omitted when the error has no cause.
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

	// RemoteError carries its own Code / Message / Case / Cause taxonomy.
	// Read them directly from re; read protocol-level remote details
	// from re.ErrResp().
	var re *apperror.RemoteError
	if errors.As(err, &re) {
		if resp := re.ErrResp(); resp != nil {
			attrs = append(attrs, slog.Int("status", resp.StatusCode()))
			if resp.BodyCode != "" {
				attrs = append(attrs, slog.String("body_code", resp.BodyCode))
			}
			if resp.BodyMessage != "" {
				attrs = append(attrs, slog.String("body_message", resp.BodyMessage))
			}
			if resp.RetryAfter > 0 {
				attrs = append(attrs, slog.Duration("retry_after", resp.RetryAfter))
			}
		}
		attrs = append(attrs,
			slog.String("code", re.Code().Name()),
			slog.String("message", re.Message()),
		)
		if c := re.Case(); c != nil {
			attrs = append(attrs, slog.String("case", c.Identifier()))
		}
		if cause := re.Cause(); cause != nil {
			attrs = append(attrs, slog.String("cause", cause.Error()))
		}
		return attrs
	}

	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		return []slog.Attr{slog.String("error", err.Error())}
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
