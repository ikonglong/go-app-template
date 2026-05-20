package assembly

import (
	"go.uber.org/dig"

	"go-app-template/internal/common/clock"
	"go-app-template/internal/common/idgen"
)

// provideCommon registers the cross-cutting helpers (wall-clock, ULID
// generator) as their port interfaces. Tests substitute fakes by building
// a separate container with alternate providers; production always gets
// the real implementations.
func provideCommon(c *dig.Container) error {
	if err := c.Provide(clock.NewRealClock, dig.As(new(clock.IClock))); err != nil {
		return err
	}
	return c.Provide(idgen.NewULIDGen, dig.As(new(idgen.IIDGen)))
}
