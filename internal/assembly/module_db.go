package assembly

import (
	"database/sql"

	_ "github.com/lib/pq"
	"go.uber.org/dig"
)

// provideDB opens the *sql.DB lazily — sql.Open does not dial — so
// /health/alive stays answerable even when Postgres is down (readiness
// owns that signal). Close is invoked from Run on graceful shutdown.
func provideDB(c *dig.Container) error {
	return c.Provide(func(cfg Config) (*sql.DB, error) {
		return sql.Open("postgres", cfg.DSN())
	})
}
