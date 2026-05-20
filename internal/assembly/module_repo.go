package assembly

import (
	"database/sql"

	"go.uber.org/dig"

	"go-app-template/internal/adapter/repo/jet"
	jetrec "go-app-template/internal/adapter/repo/jet/gen/test/public/record"
	"go-app-template/internal/domain"
)

// provideRepo wires the go-jet persistence adapter. The sqlc adapter is
// the parallel option kept around for benchmarking / migration (see
// internal/adapter/repo/sqlc) — pick one per binary; switching is a
// one-line edit of this provider.
//
// dig.As is used twice: once to expose AccountMapper only as its port
// interface, and once to expose AccountRepo as domain.IAccountRepo so
// application.NewSignUpCmd / NewSignInCmd resolve against the interface,
// not the concrete type.
func provideRepo(c *dig.Container) error {
	if err := c.Provide(
		jet.NewAccountMapper,
		dig.As(new(jet.IMapper[domain.Account, jetrec.Account])),
	); err != nil {
		return err
	}
	return c.Provide(
		func(db *sql.DB, m jet.IMapper[domain.Account, jetrec.Account]) *jet.AccountRepo {
			return jet.NewAccountRepo(db, m)
		},
		dig.As(new(domain.IAccountRepo)),
	)
}
