package assembly

import (
	"go.uber.org/dig"

	"go-app-template/internal/adapter/passwordhash"
	"go-app-template/internal/application"
)

// providePasswordHasher exposes the bcrypt implementation as the
// application-layer port. Cost comes from Config so it can be tuned per
// environment without re-wiring constructors.
func providePasswordHasher(c *dig.Container) error {
	return c.Provide(
		func(cfg Config) *passwordhash.BcryptHasher {
			return passwordhash.NewBcryptHasher(cfg.BcryptCost)
		},
		dig.As(new(application.IPasswordHasher)),
	)
}
