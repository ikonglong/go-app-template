package log

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// TestCtxHandlerInjectsRequestID proves Approach B end to end: a request_id
// stored as a ctx VALUE (no bound logger) reaches the record because
// ctxHandler reads it at Handle time.
func TestCtxHandlerInjectsRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newCtxHandler(slog.NewJSONHandler(&buf, nil)))

	ctx := WithRequestID(context.Background(), "req-123")
	logger.InfoContext(ctx, "hello")

	if got := logField(t, buf.Bytes(), "request_id"); got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
}

// TestCtxHandlerNoRequestID confirms the handler is a no-op when ctx carries
// no request_id — which is what makes installing it unconditionally safe for
// Approach A (no double-counting).
func TestCtxHandlerNoRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newCtxHandler(slog.NewJSONHandler(&buf, nil)))

	logger.InfoContext(context.Background(), "hello")

	if got := logField(t, buf.Bytes(), "request_id"); got != nil {
		t.Fatalf("request_id = %v, want absent", got)
	}
}

// TestCtxHandlerSurvivesWith proves the decorator re-wraps on .With so a
// derived logger still injects from ctx (the reason WithAttrs is overridden).
func TestCtxHandlerSurvivesWith(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newCtxHandler(slog.NewJSONHandler(&buf, nil))).
		With(slog.String("component", "test"))

	logger.InfoContext(WithRequestID(context.Background(), "req-9"), "hello")

	if got := logField(t, buf.Bytes(), "request_id"); got != "req-9" {
		t.Fatalf("request_id = %v, want req-9 (decorator lost on .With?)", got)
	}
}

func logField(t *testing.T, line []byte, key string) any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil {
		t.Fatalf("unmarshal log line %q: %v", line, err)
	}
	return rec[key]
}
