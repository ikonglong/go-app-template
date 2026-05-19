// Package sys holds adapters for ambient process concerns the application
// depends on through ports — wall-clock time and ID generation today, and
// any future "everything outside the program" inputs that don't warrant
// their own package.
package sys

import "time"

// RealClock returns the host clock's current UTC time. Tests substitute a
// fixed-time fake.
type RealClock struct{}

func NewRealClock() *RealClock { return &RealClock{} }

func (RealClock) Now() time.Time { return time.Now().UTC() }
