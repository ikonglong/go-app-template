package log

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/ikonglong/go-apperror"
)

// stackValue extracts the [][]string carried by the single "stack" attr
// StackAttrs is expected to return, failing the test otherwise.
func stackValue(t *testing.T, attrs []slog.Attr) [][]string {
	t.Helper()
	if len(attrs) != 1 {
		t.Fatalf("want exactly one attr, got %d: %v", len(attrs), attrs)
	}
	if attrs[0].Key != "stack" {
		t.Fatalf("want attr key %q, got %q", "stack", attrs[0].Key)
	}
	frames, ok := attrs[0].Value.Any().([][]string)
	if !ok {
		t.Fatalf("want stack value [][]string, got %T", attrs[0].Value.Any())
	}
	return frames
}

func TestStackAttrsEmittedForUnexpectedCode(t *testing.T) {
	err := apperror.NewInternal("user.lookup") // origin: this line

	frames := stackValue(t, StackAttrs(err))
	if len(frames) == 0 || len(frames[0]) == 0 {
		t.Fatalf("expected a non-empty stack, got %v", frames)
	}
	if !strings.Contains(frames[0][0], "TestStackAttrsEmittedForUnexpectedCode") {
		t.Errorf("origin frame = %q; want it to name the constructing test func", frames[0][0])
	}
}

func TestStackAttrsSkippedForExpectedCode(t *testing.T) {
	// NotFound is routine control flow — no stack should be logged even
	// though the AppError still captured one internally.
	if attrs := StackAttrs(apperror.NewNotFound("user.lookup")); attrs != nil {
		t.Errorf("expected no stack attr for NotFound, got %v", attrs)
	}
}

func TestStackAttrsSkippedForNonAppError(t *testing.T) {
	if attrs := StackAttrs(errors.New("plain")); attrs != nil {
		t.Errorf("expected no stack attr for a plain error, got %v", attrs)
	}
	if attrs := StackAttrs(nil); attrs != nil {
		t.Errorf("expected no stack attr for nil, got %v", attrs)
	}
}

func TestStackAttrsWalksWrappedAppError(t *testing.T) {
	// A %w wrapper around an unexpected AppError: the AppError layer still
	// gets found and its stack emitted.
	wrapped := fmt.Errorf("serving request: %w", apperror.NewIllegalState("order.invariant"))
	frames := stackValue(t, StackAttrs(wrapped))
	if len(frames) == 0 {
		t.Fatal("expected the wrapped AppError's stack to be emitted")
	}
}

func TestErrGroupNestsStackForUnexpectedCode(t *testing.T) {
	g := ErrGroup(apperror.NewInternal("user.lookup"))
	if g.Key != "err" {
		t.Fatalf("want group key %q, got %q", "err", g.Key)
	}
	var hasStack bool
	for _, a := range g.Value.Group() {
		if a.Key == "stack" {
			hasStack = true
		}
	}
	if !hasStack {
		t.Error("expected err group to nest a stack field for an InternalError")
	}
}

func TestErrGroupOmitsStackForExpectedCode(t *testing.T) {
	g := ErrGroup(apperror.NewNotFound("user.lookup"))
	for _, a := range g.Value.Group() {
		if a.Key == "stack" {
			t.Error("expected no stack field in err group for a NotFound")
		}
	}
}
