package log

import (
	"errors"
	"log/slog"

	"github.com/ikonglong/go-apperror"
)

// stackWorthyCodes are the Codes whose failures are "unexpected" — internal
// faults and broken invariants, i.e. the 500-class errors where a stack
// trace earns its place in the log. Expected, control-flow errors
// (NotFound, AlreadyExists, IllegalInput, ...) are deliberately excluded so
// routine failures don't bury logs in stack noise.
//
// This is the "capture always, render selectively" knob from the stacktrace
// design: AppError records a stack at every construction, but we only emit
// it for codes a human would actually open a trace for.
var stackWorthyCodes = map[apperror.Code]bool{
	apperror.CodeInternal:     true,
	apperror.CodeUnknown:      true,
	apperror.CodeIllegalState: true,
}

// StackAttrs returns a single "stack" structured-log attr carrying the
// captured call stacks of the error chain, or nil when no stack should be
// logged.
//
// A stack is emitted only when the chain's primary *AppError carries an
// unexpected (500-class) Code — see stackWorthyCodes. When it is, every
// *AppError layer in the chain that has a captured stack contributes one
// entry (outermost layer first, matching errors.Unwrap order); each entry
// is that layer's list of "file:line function" frames, innermost frame
// first. The value is a [][]string with no embedded newlines, so it renders
// safely under both the JSON and text handlers and never splits a log
// record.
//
// Kept separate from ErrAttrs on purpose: ErrAttrs always emits the error's
// identifying fields (code/case/message); the stack is an optional,
// code-gated diagnostic layered on top by the inbound boundary.
func StackAttrs(err error) []slog.Attr {
	if err == nil {
		return nil
	}

	// Only AppError-led chains get a stack. When a RemoteError is the
	// cause, the wrapping AppError's stack points at the adapter translation
	// site rather than a useful origin, and the remote fields already come
	// from ErrAttrs — so skip the stack whenever a RemoteError is in play.
	var primary *apperror.AppError
	if !errors.As(err, &primary) {
		return nil
	}
	var re *apperror.RemoteError
	if errors.As(err, &re) {
		return nil
	}
	if !stackWorthyCodes[primary.Code()] {
		return nil
	}

	var stacks [][]string
	for cur := err; cur != nil; cur = errors.Unwrap(cur) {
		// Inspect the concrete type of THIS layer; errors.As would skip
		// past the layer we want to read the stack from.
		ae, ok := cur.(*apperror.AppError) //nolint:errorlint
		if !ok {
			continue
		}
		if frames := ae.StackTrace().Strings(); len(frames) > 0 {
			stacks = append(stacks, frames)
		}
	}
	if len(stacks) == 0 {
		return nil
	}
	return []slog.Attr{slog.Any("stack", stacks)}
}
