package assembly

import (
	"go.uber.org/dig"

	"go-app-template/internal/application"
)

// provideApplication wires the use-case commands. They are the
// integration seam between the inbound REST adapter and everything
// below; dig resolves their dependencies (repo, hasher, clock, idgen)
// from the modules above by interface type.
func provideApplication(c *dig.Container) error {
	if err := c.Provide(application.NewSignUpCmd); err != nil {
		return err
	}
	return c.Provide(application.NewSignInCmd)
}
