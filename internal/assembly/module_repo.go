package assembly

import (
	"database/sql"

	// scaffold:if jet
	"github.com/go-jet/jet/v2/qrm"
	// scaffold:endif
	"go.uber.org/dig"

	// scaffold:if jet
	"go-app-template/internal/adapter/repo/jet"
	jetrec "go-app-template/internal/adapter/repo/jet/gen/test/public/record"
	// scaffold:endif
	// scaffold:if sqlc
	"go-app-template/internal/adapter/repo/sqlc"
	sqlcgen "go-app-template/internal/adapter/repo/sqlc/gen"
	// scaffold:endif
	"go-app-template/internal/domain"
)

// scaffold:begin
// provideRepo dispatches to the persistence adapter selected by
// Config.RepoImpl ("jet" or "sqlc"), read from REPO__IMPL at startup.
func provideRepo(c *dig.Container, cfg Config) error {
	if cfg.RepoImpl == "sqlc" {
		return provideSqlcRepo(c)
	}
	return provideJetRepo(c)
}

// scaffold:end

// scaffold:if jet
func provideJetDBBridge(db *sql.DB) qrm.DB { return db }

func provideJetRepo(c *dig.Container) error {
	if err := c.Provide(provideJetDBBridge); err != nil {
		return err
	}
	if err := c.Provide(
		jet.NewAccountMapper,
		dig.As(new(jet.IMapper[domain.Account, jetrec.Account])),
	); err != nil {
		return err
	}
	return c.Provide(jet.NewAccountRepo, dig.As(new(domain.IAccountRepo)))
}

// scaffold:endif

// scaffold:if sqlc
func provideSqlcDBBridge(db *sql.DB) sqlcgen.DBTX { return db }

func provideSqlcRepo(c *dig.Container) error {
	if err := c.Provide(provideSqlcDBBridge); err != nil {
		return err
	}
	if err := c.Provide(
		sqlc.NewAccountMapper,
		dig.As(new(sqlc.IMapper[domain.Account, sqlcgen.Account])),
	); err != nil {
		return err
	}
	return c.Provide(sqlc.NewAccountRepo, dig.As(new(domain.IAccountRepo)))
}

// scaffold:endif
