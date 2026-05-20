package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// hlogBridge adapts hlog.FullLogger onto our *slog.Logger so Hertz's own
// framework lines flow through the SAME handler as application logs — same
// format (text/json), same destinations, same ctxHandler enrichment.
//
// It exists because hertz-contrib/logger/slog hardcodes a JSON handler
// (slog.NewJSONHandler, no hook to swap it — confirmed against v1.0.0, the
// only released version) and so would emit JSON even when we run text
// locally, breaking the one thing this is meant to guarantee: uniform
// output across our lines and the framework's.
//
// hlog's seven levels collapse onto slog's four: Trace→Debug, Notice→Info,
// Fatal→Error. This is deliberate — mapping onto standard slog levels keeps
// the rendered level names standard (INFO, ERROR) instead of slog's offset
// rendering of hlog's custom numeric levels ("INFO+2", "ERROR+4"), which is
// what makes framework lines read uniformly with ours. Fatal does NOT exit
// the process (mirroring hertzslog): a logging bridge only logs.
type hlogBridge struct {
	l     *slog.Logger
	level *slog.LevelVar // shared with the app handler: one level knob per process
}

var _ hlog.FullLogger = (*hlogBridge)(nil)

// NewHlogBridge builds the hlog.FullLogger backing Hertz. Pass the logger
// and options returned by Init; the bridge reuses Init's *slog.LevelVar so
// hlog.SetLevel and LOG__LEVEL move the same unified level.
func NewHlogBridge(l *slog.Logger, opts *slog.HandlerOptions) hlog.FullLogger {
	b := &hlogBridge{l: l}
	if lv, ok := opts.Level.(*slog.LevelVar); ok {
		b.level = lv
	}
	return b
}

func (b *hlogBridge) log(level hlog.Level, v ...any) {
	b.l.Log(context.Background(), hlogToSlog(level), fmt.Sprint(v...))
}

func (b *hlogBridge) logf(level hlog.Level, format string, v ...any) {
	b.l.Log(context.Background(), hlogToSlog(level), fmt.Sprintf(format, v...))
}

func (b *hlogBridge) ctxLogf(ctx context.Context, level hlog.Level, format string, v ...any) {
	b.l.Log(ctx, hlogToSlog(level), fmt.Sprintf(format, v...))
}

// Logger
func (b *hlogBridge) Trace(v ...any)  { b.log(hlog.LevelTrace, v...) }
func (b *hlogBridge) Debug(v ...any)  { b.log(hlog.LevelDebug, v...) }
func (b *hlogBridge) Info(v ...any)   { b.log(hlog.LevelInfo, v...) }
func (b *hlogBridge) Notice(v ...any) { b.log(hlog.LevelNotice, v...) }
func (b *hlogBridge) Warn(v ...any)   { b.log(hlog.LevelWarn, v...) }
func (b *hlogBridge) Error(v ...any)  { b.log(hlog.LevelError, v...) }
func (b *hlogBridge) Fatal(v ...any)  { b.log(hlog.LevelFatal, v...) }

// FormatLogger
func (b *hlogBridge) Tracef(format string, v ...any)  { b.logf(hlog.LevelTrace, format, v...) }
func (b *hlogBridge) Debugf(format string, v ...any)  { b.logf(hlog.LevelDebug, format, v...) }
func (b *hlogBridge) Infof(format string, v ...any)   { b.logf(hlog.LevelInfo, format, v...) }
func (b *hlogBridge) Noticef(format string, v ...any) { b.logf(hlog.LevelNotice, format, v...) }
func (b *hlogBridge) Warnf(format string, v ...any)   { b.logf(hlog.LevelWarn, format, v...) }
func (b *hlogBridge) Errorf(format string, v ...any)  { b.logf(hlog.LevelError, format, v...) }
func (b *hlogBridge) Fatalf(format string, v ...any)  { b.logf(hlog.LevelFatal, format, v...) }

// CtxLogger — ctx carries request-scoped values the ctxHandler injects.
func (b *hlogBridge) CtxTracef(ctx context.Context, format string, v ...any) {
	b.ctxLogf(ctx, hlog.LevelTrace, format, v...)
}
func (b *hlogBridge) CtxDebugf(ctx context.Context, format string, v ...any) {
	b.ctxLogf(ctx, hlog.LevelDebug, format, v...)
}
func (b *hlogBridge) CtxInfof(ctx context.Context, format string, v ...any) {
	b.ctxLogf(ctx, hlog.LevelInfo, format, v...)
}
func (b *hlogBridge) CtxNoticef(ctx context.Context, format string, v ...any) {
	b.ctxLogf(ctx, hlog.LevelNotice, format, v...)
}
func (b *hlogBridge) CtxWarnf(ctx context.Context, format string, v ...any) {
	b.ctxLogf(ctx, hlog.LevelWarn, format, v...)
}
func (b *hlogBridge) CtxErrorf(ctx context.Context, format string, v ...any) {
	b.ctxLogf(ctx, hlog.LevelError, format, v...)
}
func (b *hlogBridge) CtxFatalf(ctx context.Context, format string, v ...any) {
	b.ctxLogf(ctx, hlog.LevelFatal, format, v...)
}

// Control

// SetLevel retunes the unified process level (shared with app logs).
func (b *hlogBridge) SetLevel(level hlog.Level) {
	if b.level != nil {
		b.level.Set(hlogToSlog(level))
	}
}

// SetOutput is intentionally a no-op: the destination is owned centrally by
// applog.Init / NewWriter so console/file config has a single source of
// truth. Hertz's server never calls it on a custom-installed logger (the
// only callers of the Control.SetOutput path are the package-level
// hlog.SetOutput helper and Hertz's built-in default logger, neither of
// which we route through).
func (b *hlogBridge) SetOutput(io.Writer) {}

// hlogToSlog collapses hlog's seven levels onto slog's four standard ones.
func hlogToSlog(l hlog.Level) slog.Level {
	switch l {
	case hlog.LevelTrace, hlog.LevelDebug:
		return slog.LevelDebug
	case hlog.LevelInfo, hlog.LevelNotice:
		return slog.LevelInfo
	case hlog.LevelWarn:
		return slog.LevelWarn
	default: // Error, Fatal, and any unknown value
		return slog.LevelError
	}
}
