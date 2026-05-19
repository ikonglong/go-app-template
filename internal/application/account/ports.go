// Package account contains the use cases that orchestrate the Account
// aggregate. Use cases depend on the domain aggregate/port and on the
// infrastructure ports declared in this file; concrete adapters live under
// internal/adapter/out/.
package account

import "time"

// IPasswordHasher abstracts a one-way password hashing scheme (bcrypt,
// argon2, …). Hash is called at SignUp; Compare is called at SignIn and
// must return a non-nil error on mismatch (the application treats every
// non-nil return as "wrong password" and collapses it into the same
// ErrInvalidCredentials sentinel — never leak the underlying hasher error
// to callers, that would reveal whether the account exists).
type IPasswordHasher interface {
	Hash(plain string) (string, error)
	Compare(hash, plain string) error
}

// IClock yields the current time. Injected (rather than calling time.Now
// directly) so the domain factories that take a time.Time argument can
// be exercised deterministically in tests.
type IClock interface {
	Now() time.Time
}

// IIDGen issues a fresh aggregate identity. Implementations must produce
// values that fit the storage column (account.id is VARCHAR(30)) and that
// the application is willing to expose in URLs/logs.
type IIDGen interface {
	Next() string
}
