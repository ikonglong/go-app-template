// Package application contains the use cases that orchestrate the
// domain aggregates. Each use case is a command struct with a Run method
// (see [Martin Fowler — Command-Oriented Interface]). Universal
// abstractions (Clock, IDGen) live in internal/common; the
// application-local ports declared in this file are abstractions whose
// semantics are tied to this application's needs — keep them here
// alongside the use cases that consume them.
//
// [Martin Fowler — Command-Oriented Interface]: https://martinfowler.com/bliki/CommandOrientedInterface.html
package application

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
