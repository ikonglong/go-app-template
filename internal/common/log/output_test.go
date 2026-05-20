package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Console-only resolves straight to os.Stdout (no wrapping, no-op close),
// so the common local/k8s path adds zero overhead.
func TestNewWriterConsoleOnly(t *testing.T) {
	w, closeFn, err := NewWriter(Output{Console: true})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if w != os.Stdout {
		t.Errorf("console-only writer = %T, want os.Stdout", w)
	}
	if err := closeFn(); err != nil {
		t.Errorf("console close should be a no-op, got %v", err)
	}
}

// The file sink creates missing parent dirs and actually persists writes.
func TestNewWriterFileCreatesAndWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "app.log")

	w, closeFn, err := NewWriter(Output{File: &FileSink{Path: path, MaxSizeMB: 1}})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Idempotent close — main both defers and may call it on early exit.
	if err := closeFn(); err != nil {
		t.Errorf("second close should be a no-op, got %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("log file not written: %v", err)
	}
	if !strings.Contains(string(b), "hello") {
		t.Errorf("log file = %q, want it to contain %q", b, "hello")
	}
}

// With both destinations on, the file half of the fanout still receives
// every record (the console half goes to os.Stdout and isn't asserted).
func TestNewWriterBothFanout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	w, closeFn, err := NewWriter(Output{Console: true, File: &FileSink{Path: path, MaxSizeMB: 1}})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if _, err := w.Write([]byte("dual\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("log file not written: %v", err)
	}
	if !strings.Contains(string(b), "dual") {
		t.Errorf("file half of fanout = %q, want it to contain %q", b, "dual")
	}
}

// An empty Output must not leave the process logless — it degrades to
// os.Stdout rather than discarding records.
func TestNewWriterEmptyFallsBackToStdout(t *testing.T) {
	w, closeFn, err := NewWriter(Output{})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer closeFn()
	if w != os.Stdout {
		t.Errorf("empty Output writer = %T, want os.Stdout fallback", w)
	}
}
