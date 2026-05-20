// Package idgen provides an ID generation abstraction for issuing fresh
// aggregate identities.
package idgen

import (
	cryptorand "crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// IIDGen issues a fresh aggregate identity. Implementations must produce
// values that fit the storage column (account.id is VARCHAR(30)) and that
// the application is willing to expose in URLs/logs.
type IIDGen interface {
	Next() string
}

// ULIDGen issues 26-character Crockford-base32 ULIDs. Fits the account.id
// VARCHAR(30) column with room to spare. Lexicographic order tracks time
// order, which keeps B-tree index writes near the right edge and makes
// debugging by ID much easier than purely-random schemes (UUIDv4, Nano ID).
//
// The crypto/rand entropy source is wrapped in a sync.Pool-equivalent mutex
// because ulid.MonotonicEntropy is not goroutine-safe — every call site
// (REST handler invocations) is concurrent.
type ULIDGen struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

func NewULIDGen() *ULIDGen {
	return &ULIDGen{entropy: ulid.Monotonic(cryptorand.Reader, 0)}
}

func (g *ULIDGen) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), g.entropy).String()
}
