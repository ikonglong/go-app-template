// Package clock provides a Clock abstraction so code that needs the
// current time can be injected with a fake in tests.
package clock

import "time"

// IClock yields the current time. Injected (rather than calling time.Now
// directly) so domain factories that take a time.Time argument can be
// exercised deterministically in tests.
type IClock interface {
	Now() time.Time
}

// RealClock returns the host clock's current UTC time. Tests substitute a
// fixed-time fake.
type RealClock struct{}

func NewRealClock() *RealClock { return &RealClock{} }

func (RealClock) Now() time.Time { return time.Now().UTC() }
