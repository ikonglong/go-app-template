package main

import (
	"context"
	"log/slog"
	"os"

	"go-app-template/internal/assembly"
	applog "go-app-template/internal/common/log"
)

// main is the composition root entry point. The real work lives in run so
// that deferred cleanup (closing the log file sink) executes on every exit
// path — os.Exit, which main would otherwise call directly, skips defers.
func main() {
	os.Exit(run())
}

// run loads config (incl. .env), inits logging, builds the dig container,
// and hands off to assembly.Run which registers inbound handlers and blocks
// on Hertz Spin until SIGINT/SIGTERM. Every wiring decision lives in
// internal/assembly so this file stays trivial. The int return is the
// process exit code.
func run() int {
	cfg := assembly.LoadConfig(".env")

	closeLog, err := assembly.InitLogger(cfg)
	if err != nil {
		// Logger isn't up yet; slog.Default (stderr) is the only sink.
		slog.Error("init logger failed", slog.Any("error", err))
		return 1
	}
	defer closeLog()

	container, err := assembly.BuildContainer(cfg)
	if err != nil {
		applog.ErrorAttrs(context.Background(), "assembly.build", "build container failed",
			slog.Any("error", err),
		)
		return 1
	}
	if err := container.Invoke(assembly.Run); err != nil {
		applog.ErrorAttrs(context.Background(), "assembly.run", "container invoke failed",
			slog.Any("error", err),
		)
		return 1
	}
	return 0
}
