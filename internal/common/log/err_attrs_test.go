package log

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/ikonglong/go-apperror"
)

// ErrGroup wraps the ErrAttrs fields under a single "err" key so they
// render as a nested object rather than flat alongside the record's other
// keys. These tests pin that shape and the field set.

func TestErrGroupNestsErrorFields(t *testing.T) {
	err := apperror.NewAlreadyExists("account.create",
		apperror.WithMessage("email already registered"),
		apperror.WithCase(apperror.NewStrCase("email_taken")),
	)

	g := ErrGroup(err)
	if g.Key != "err" {
		t.Fatalf("group key = %q, want %q", g.Key, "err")
	}
	if g.Value.Kind() != slog.KindGroup {
		t.Fatalf("value kind = %v, want Group", g.Value.Kind())
	}

	got := groupToMap(g)
	if want := err.Code().Name(); got["code"] != want {
		t.Errorf("code = %q, want %q", got["code"], want)
	}
	if got["message"] != "email already registered" {
		t.Errorf("message = %q", got["message"])
	}
	if got["case"] != "email_taken" {
		t.Errorf("case = %q", got["case"])
	}
}

// cause is the wrapped root error. It must appear in the log group (so the
// server keeps the root cause) even though the inbound boundary never puts
// it in the response body — guide §3.4.
func TestErrGroupIncludesCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := apperror.NewUnavailable("health.ready",
		apperror.WithMessage("database not ready"),
		apperror.WithCause(cause),
	)

	got := groupToMap(ErrGroup(err))
	if got["cause"] != "connection refused" {
		t.Errorf("cause = %q, want %q", got["cause"], "connection refused")
	}
}

// A canonical AppError wrapping a RemoteError as its cause (the go-apperror
// v0.2.0 driven-adapter shape) must surface BOTH the remote-side fields
// (service/operation/status/body_code/body_message/retry_after, recovered via
// errors.As(&re)) and the canonical code/message (from the wrapping AppError,
// recovered via errors.As(&ae)) in one flat field set.
func TestErrAttrsSurfacesWrappedRemoteError(t *testing.T) {
	re := &apperror.RemoteError{
		Service:     "user-service",
		Operation:   "GetUser",
		Response:    &apperror.Response{StatusCode: 503, Body: []byte(`{"code":"DEGRADED"}`)},
		BodyCode:    "DEGRADED",
		BodyMessage: "service in maintenance",
		RetryAfter:  30 * time.Second,
	}
	err := apperror.NewUnavailable("user-service.GetUser",
		apperror.WithMessage("user-service degraded"),
		apperror.WithCause(re),
	)

	got := groupToMap(ErrGroup(err))
	for k, want := range map[string]string{
		"service":      "user-service",
		"operation":    "GetUser",
		"status":       "503",
		"body_code":    "DEGRADED",
		"body_message": "service in maintenance",
		"retry_after":  "30s",
		"code":         apperror.CodeUnavailable.Name(),
		"message":      "user-service degraded",
	} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
}

// A RemoteError anywhere in the chain suppresses the stack, even when the
// canonical code is otherwise stack-worthy (Unknown here): the wrapping
// AppError's stack points at the adapter translation site, not a useful
// origin, and the remote fields already carry the diagnostic signal.
func TestStackAttrsSkippedWhenRemoteErrorWrapped(t *testing.T) {
	re := &apperror.RemoteError{
		Service:   "user-service",
		Operation: "GetUser",
		Response:  &apperror.Response{StatusCode: 500},
	}
	err := apperror.NewUnknown("user-service.GetUser", apperror.WithCause(re))

	if attrs := StackAttrs(err); attrs != nil {
		t.Errorf("expected no stack attr when a RemoteError is wrapped, got %v", attrs)
	}
}

// A nil err yields an empty group, which slog must omit entirely — no
// "err":{} noise in the output.
func TestErrGroupEmptyOmittedByHandler(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf, "json", "debug")

	ErrorAttrs(context.Background(), "x.y", "msg", ErrGroup(nil))

	rec := decodeRecord(t, &buf)
	if _, ok := rec["err"]; ok {
		t.Errorf("empty err group should be omitted, got %v", rec["err"])
	}
}

// The whole-record shape the REST boundary emits: event stays at the top
// level (primary aggregation key), while err and req are sibling nested
// objects.
func TestLogShapeEventTopLevelGroupsNested(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf, "json", "debug")

	err := apperror.NewIllegalInput("account.create", apperror.WithMessage("bad input"))
	ErrorAttrs(context.Background(), "account.create", "request failed",
		ErrGroup(err),
		slog.Group("req",
			slog.String("method", "POST"),
			slog.String("url", "/accounts"),
		),
	)

	rec := decodeRecord(t, &buf)
	if rec["event"] != "account.create" {
		t.Errorf("event = %v, want top-level account.create", rec["event"])
	}

	errObj, ok := rec["err"].(map[string]any)
	if !ok {
		t.Fatalf("err is not a nested object: %T", rec["err"])
	}
	if errObj["code"] != err.Code().Name() {
		t.Errorf("err.code = %v, want %v", errObj["code"], err.Code().Name())
	}

	reqObj, ok := rec["req"].(map[string]any)
	if !ok {
		t.Fatalf("req is not a nested object: %T", rec["req"])
	}
	if reqObj["method"] != "POST" || reqObj["url"] != "/accounts" {
		t.Errorf("req = %v, want {method:POST url:/accounts}", reqObj)
	}
}

func groupToMap(a slog.Attr) map[string]string {
	m := make(map[string]string)
	for _, g := range a.Value.Group() {
		m[g.Key] = g.Value.String()
	}
	return m
}

func decodeRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal log record: %v\nraw: %s", err, buf.String())
	}
	return rec
}
