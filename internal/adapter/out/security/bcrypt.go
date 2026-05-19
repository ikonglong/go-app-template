// Package security holds adapters for cryptographic operations the
// application depends on through ports (currently: password hashing).
package security

import "golang.org/x/crypto/bcrypt"

// BcryptHasher implements account.IPasswordHasher with bcrypt.
//
// Cost picks the work factor per hash; 12 is the common 2025-era default
// (≈ 250 ms on a modern x86 core) — slow enough to make offline cracking
// painful, fast enough to keep signup/signin under typical request budgets.
// Bump when hardware improves.
type BcryptHasher struct {
	Cost int
}

func NewBcryptHasher(cost int) *BcryptHasher {
	return &BcryptHasher{Cost: cost}
}

func (h *BcryptHasher) Hash(plain string) (string, error) {
	out, err := bcrypt.GenerateFromPassword([]byte(plain), h.Cost)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Compare returns nil on match and bcrypt.ErrMismatchedHashAndPassword on
// mismatch. Callers (the application layer) collapse any non-nil return
// into ErrInvalidCredentials; the specific bcrypt error is intentionally
// not exposed past this boundary.
func (h *BcryptHasher) Compare(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
