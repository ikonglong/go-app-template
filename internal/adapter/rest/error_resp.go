package rest

import (
	"context"
	"errors"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/ikonglong/go-apperror"

	applog "go-app-template/internal/common/log"
)

// errorResp is the wire shape for failures. Sanitized:
// Code/Case/Message only, no Cause, no stack trace, no upstream
// transport details. Case is omitted from the body when nil so the
// field doesn't appear as `"case": ""`.
type errorResp struct {
	Code    string `json:"code"`
	Case    string `json:"case,omitempty"`
	Message string `json:"message"`
}

// renderError translates any error from below into a JSON failure body
// plus the matching HTTP status. It is the single boundary at which Code
// is mapped to a status code; handlers never call c.JSON with an HTTP
// status themselves on the failure path.
//
// Any non-AppError caught here is wrapped as Internal — that is by
// design: the only way an error of unknown shape can leak this far is a
// bug or an un-translated infrastructure error, both of which deserve
// a generic 500 rather than a leaked stack trace.
func renderError(ctx context.Context, c *app.RequestContext, err error) {
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		appErr = apperror.NewInternal("rest.unhandled", apperror.WithCause(err))
	}

	// Error and request context each go in as a nested group ("err" / "req")
	// rather than flat keys, so each stays namespaced and they don't
	// intermix with one another or with the record's top-level event/msg.
	// url is the RequestURI (path + query) — scheme/host are constant noise
	// here; the query may carry sensitive params, so drop to c.Path() if
	// that ever becomes a concern.
	applog.ErrorAttrs(ctx, appErr.Event(), "request failed",
		applog.ErrGroup(err),
		slog.Group("req",
			slog.String("method", string(c.Method())),
			slog.String("url", string(c.URI().RequestURI())),
		),
	)

	status, ok := apperror.HTTPStatusFor(appErr.Code())
	if !ok {
		status = apperror.StatusInternalServerError
	}

	body := errorResp{
		Code:    appErr.Code().Name(),
		Message: appErr.Message(),
	}
	if cs := appErr.Case(); cs != nil {
		body.Case = cs.Identifier()
	}
	c.JSON(status.Value(), body)
}
