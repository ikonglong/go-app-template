package assembly

import (
	"database/sql"

	"github.com/cloudwego/hertz/pkg/app/server"
	"go.uber.org/dig"

	"go-app-template/internal/adapter/rest"
)

// provideRest constructs the Hertz instance and every inbound handler.
// Middleware that must apply to every endpoint (RequestLogger) is
// installed here — registering it on the engine before any route is what
// makes /health/* and the application routes share the same access-log
// pipeline.
//
// HealthCheckHandler takes its DB-ping as a plain function rather than a
// full repository port (see internal/adapter/rest/health_check.go for
// the reasoning); the lambda below adapts *sql.DB.PingContext to that
// shape without leaking the DB into the handler's import graph.
func provideRest(c *dig.Container) error {
	if err := c.Provide(func(cfg Config) *server.Hertz {
		h := server.Default(server.WithHostPorts(cfg.Addr()))
		h.Use(rest.RequestLogger())
		return h
	}); err != nil {
		return err
	}
	if err := c.Provide(func(db *sql.DB) *rest.HealthCheckHandler {
		return rest.NewHealthCheckHandler(db.PingContext)
	}); err != nil {
		return err
	}
	if err := c.Provide(rest.NewAccountHandler); err != nil {
		return err
	}
	return c.Provide(rest.NewSessionHandler)
}
