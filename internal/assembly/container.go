// Package assembly is the application's composition root. It owns the
// wiring graph: every concrete adapter, every application command, the
// Hertz instance, and the lifecycle that ties them together.
//
// Layout: container.go is the entry point; each module_<capability>.go
// file registers one capability's providers on the dig container.
// cmd/server/main.go does nothing but Config load → logger init →
// BuildContainer → Invoke(Run).
package assembly

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app/server"
	"go.uber.org/dig"

	"go-app-template/internal/adapter/rest"
	applog "go-app-template/internal/common/log"
)

// Event constants for the server-lifecycle log lines. Pure operation
// names; outcome (success / failure) lives in the log level + msg.
const (
	eventServerListen   = "server.listen"
	eventServerShutdown = "server.shutdown"
)

// BuildContainer wires every provider needed to run the HTTP server. The
// returned container has no external mutable state — callers consume it
// via Invoke. Provider order doesn't matter (dig resolves the graph at
// Invoke time); the call sequence below only fixes the order in which
// registration errors surface.
func BuildContainer(cfg Config) (*dig.Container, error) {
	c := dig.New()
	if err := c.Provide(func() Config { return cfg }); err != nil {
		return nil, fmt.Errorf("provide config: %w", err)
	}
	for _, register := range []func(*dig.Container) error{
		provideCommon,
		provideDB,
		func(c *dig.Container) error { return provideRepo(c, cfg) },
		providePasswordHasher,
		provideApplication,
		provideRest,
	} {
		if err := register(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// runIn collects every terminal value Run needs at startup. Missing one
// surfaces as a precise "type X is not in the graph" error at Invoke time
// instead of a nil-pointer crash at request time.
type runIn struct {
	dig.In

	Cfg         Config
	Hertz       *server.Hertz
	DB          *sql.DB
	HealthCheck *rest.HealthCheckHandler
	Account     *rest.AccountHandler
	Session     *rest.SessionHandler
}

// Run is the dig.Invoke target. It registers every inbound handler on
// the Hertz instance and blocks in Spin until SIGINT/SIGTERM (Hertz owns
// signal handling internally). The DB is closed on the way out — sql.Open
// is lazy, so an early Invoke failure that never reached Run leaks
// nothing.
func Run(in runIn) error {
	in.HealthCheck.Register(in.Hertz)
	in.Account.Register(in.Hertz)
	in.Session.Register(in.Hertz)

	applog.InfoAttrs(context.Background(), eventServerListen, "HTTP server listening",
		slog.String("addr", in.Cfg.Addr()),
	)
	in.Hertz.Spin()

	if err := in.DB.Close(); err != nil {
		applog.WarnAttrs(context.Background(), eventServerShutdown, "db close failed",
			slog.Any("error", err),
		)
	}
	return nil
}
