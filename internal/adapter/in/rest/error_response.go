package rest

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/ikonglong/go-apperror"
)

// errorResponse is the wire shape for failures. Sanitized per
// error_handling_guide.md §3.4 — Code/Case/Message only, no Cause, no
// stack trace, no upstream transport details. Case is omitted from the
// body when nil so the field doesn't appear as `"case": ""`.
type errorResponse struct {
	Code    string `json:"code"`
	Case    string `json:"case,omitempty"`
	Message string `json:"message"`
}

// renderError translates any error from below into a JSON failure body
// plus the matching HTTP status. It is the single boundary at which Code
// is mapped to a status code; handlers never call c.JSON with an HTTP
// status themselves on the failure path.
//
// Any non-AppError caught here is wrapped as InternalError — that is by
// design: the only way an error of unknown shape can leak this far is a
// bug or an un-translated infrastructure error, both of which deserve
// a generic 500 rather than a leaked stack trace.
func renderError(ctx context.Context, c *app.RequestContext, err error) {
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		appErr = apperror.NewInternalError("rest.unhandled", apperror.WithCause(err))
	}

	hlog.CtxErrorf(ctx, "request failed: %v", err)

	status, ok := apperror.HTTPStatusFor(appErr.Code())
	if !ok {
		status = apperror.StatusInternalServerError
	}

	body := errorResponse{
		Code:    appErr.Code().Name(),
		Message: appErr.Message(),
	}
	if cs := appErr.Case(); cs != nil {
		body.Case = cs.Identifier()
	}
	c.JSON(status.Value(), body)
}
