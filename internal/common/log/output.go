package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// FileSink configures a rolling file destination, backed by lumberjack.
// Zero MaxBackups / MaxAgeDays mean "no limit on that axis" (lumberjack's
// own semantics); MaxSizeMB of 0 lets lumberjack apply its 100 MB default.
type FileSink struct {
	Path       string // log file path; parent dirs are created on init
	MaxSizeMB  int    // rotate once the active file reaches this size
	MaxBackups int    // max rotated files to retain (0 = keep all)
	MaxAgeDays int    // max age of rotated files in days (0 = no age limit)
	Compress   bool   // gzip rotated files
}

// Output describes where log records go. Console and File are independent
// switches: enable either, both, or — as a guarded fallback — neither.
type Output struct {
	Console bool
	File    *FileSink // nil disables the file destination
}

// NewWriter builds the io.Writer fanning records out to the configured
// destinations, plus a Close func that releases the file sink (a no-op
// when no file is configured). With both destinations on, writes go
// through an io.MultiWriter so every record lands in both, byte-identical.
//
// Console is os.Stdout (not Stderr): structured logs are the app's primary
// output, and stdout is what container runtimes capture by default.
//
// An empty Output (neither destination) falls back to os.Stdout — a
// misconfiguration should leave the process noisy, never silently logless.
//
// The file destination is validated eagerly: its parent directory is
// created here so a bad/unwritable path fails at startup rather than at the
// first log line. Close is idempotent (lumberjack tolerates a double
// Close), so callers may both defer it and call it on an early-exit path.
func NewWriter(out Output) (io.Writer, func() error, error) {
	noop := func() error { return nil }

	var writers []io.Writer
	closeFn := noop

	if out.Console {
		writers = append(writers, os.Stdout)
	}
	if out.File != nil {
		if dir := filepath.Dir(out.File.Path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("create log dir %q: %w", dir, err)
			}
		}
		lj := &lumberjack.Logger{
			Filename:   out.File.Path,
			MaxSize:    out.File.MaxSizeMB,
			MaxBackups: out.File.MaxBackups,
			MaxAge:     out.File.MaxAgeDays,
			Compress:   out.File.Compress,
		}
		writers = append(writers, lj)
		closeFn = lj.Close
	}

	switch len(writers) {
	case 0:
		return os.Stdout, noop, nil
	case 1:
		return writers[0], closeFn, nil
	default:
		return io.MultiWriter(writers...), closeFn, nil
	}
}
