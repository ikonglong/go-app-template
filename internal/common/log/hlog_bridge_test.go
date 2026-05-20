package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// The whole point of the bridge: Hertz lines follow LOG__FORMAT. text →
// logfmt, json → a JSON object — the same handler our app logs use.
func TestHlogBridgeFollowsTextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger, opts := Init(&buf, "text", "debug")
	NewHlogBridge(logger, opts).Infof("listening on %s", ":8080")

	out := buf.String()
	if !strings.Contains(out, "level=INFO") || !strings.Contains(out, `msg="listening on :8080"`) {
		t.Errorf("text bridge output = %q", out)
	}
}

func TestHlogBridgeFollowsJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger, opts := Init(&buf, "json", "debug")
	NewHlogBridge(logger, opts).Info("ready")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("json bridge output not parseable: %v\nraw: %s", err, buf.String())
	}
	if rec["level"] != "INFO" || rec["msg"] != "ready" {
		t.Errorf("record = %v", rec)
	}
}

// hlog's extra levels collapse onto standard slog names — no "INFO+2" /
// "ERROR+4" from slog's offset rendering of custom numeric levels.
func TestHlogBridgeLevelNamesAreStandard(t *testing.T) {
	for _, tc := range []struct {
		emit func(hlog.FullLogger)
		want string
	}{
		{func(l hlog.FullLogger) { l.Notice("n") }, "INFO"},
		{func(l hlog.FullLogger) { l.Fatal("f") }, "ERROR"},
		{func(l hlog.FullLogger) { l.Trace("t") }, "DEBUG"},
	} {
		var buf bytes.Buffer
		logger, opts := Init(&buf, "json", "debug")
		tc.emit(NewHlogBridge(logger, opts))

		var rec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
			t.Fatalf("unmarshal: %v\nraw: %s", err, buf.String())
		}
		if rec["level"] != tc.want {
			t.Errorf("level = %v, want %v (raw: %s)", rec["level"], tc.want, buf.String())
		}
	}
}

// SetLevel retunes the shared level var, so a previously-filtered line then
// shows up.
func TestHlogBridgeSetLevelFilters(t *testing.T) {
	var buf bytes.Buffer
	logger, opts := Init(&buf, "json", "info")
	b := NewHlogBridge(logger, opts)

	b.Debugf("hidden")
	if buf.Len() != 0 {
		t.Fatalf("debug line should be filtered at info level, got %q", buf.String())
	}

	b.SetLevel(hlog.LevelDebug)
	b.Debugf("now visible")
	if !strings.Contains(buf.String(), "now visible") {
		t.Errorf("after SetLevel(debug), debug line missing: %q", buf.String())
	}
}

// Fatal must only log, never exit — reaching the assertion after the call
// is the test (an os.Exit would take the whole test binary down).
func TestHlogBridgeFatalDoesNotExit(t *testing.T) {
	var buf bytes.Buffer
	logger, opts := Init(&buf, "json", "debug")
	NewHlogBridge(logger, opts).Fatal("boom")

	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("fatal line not logged: %q", buf.String())
	}
}
