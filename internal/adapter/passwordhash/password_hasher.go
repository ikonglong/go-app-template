// Package passwordhash holds adapters that implement the application's
// IPasswordHasher port (one-way password hashing).
package passwordhash

import (
	"github.com/ikonglong/go-apperror"
	"golang.org/x/crypto/bcrypt"

	"go-app-template/internal/application"
)

// PasswordHasher implements application.IPasswordHasher with bcrypt.
//
// Cost picks the work factor per hash; 12 is the common 2025-era default
// (≈ 250 ms on a modern x86 core) — slow enough to make offline cracking
// painful, fast enough to keep signup/signin under typical request budgets.
// Bump when hardware improves.
type PasswordHasher struct {
	Cost int
}

func NewPasswordHasher(cost int) *PasswordHasher {
	return &PasswordHasher{Cost: cost}
}

func (h *PasswordHasher) Hash(plain string) (string, error) {
	out, err := bcrypt.GenerateFromPassword([]byte(plain), h.Cost)
	if err != nil {
		return "", apperror.NewInternal("passwordhash.hash",
			apperror.WithMessage("hashing password"),
			apperror.WithCause(err))
	}
	return string(out), nil
}

// Compare returns nil on match and bcrypt.ErrMismatchedHashAndPassword on
// mismatch. Callers (the application layer) collapse any non-nil return
// into ErrInvalidCredentials; the specific bcrypt error is intentionally
// not exposed past this boundary.
func (h *PasswordHasher) Compare(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

// Compile-time check that *PasswordHasher satisfies the IPasswordHasher port.
var _ application.IPasswordHasher = (*PasswordHasher)(nil)
