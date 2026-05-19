package domain

import "context"

// IRepo is the generic outbound port for an aggregate keyed by a string ID.
// It models a DDD collection-style repository — Add / Update / Delete plus
// FindByID / MustGet. Adapters embed a concrete implementation and layer
// aggregate-specific finders (e.g. FindByEmail) on top.
//
// Error / outcome contract:
//
//   - Finders (FindByID, FindByEmail, …) return (nil, nil) when no row
//     matches. The absence of a row is a normal lookup outcome, not an
//     error — the error channel is reserved for operational failures
//     (DB unreachable, query syntax error, …). Callers do a nil check;
//     they do NOT need errors.Is(err, ErrXxxNotFound) to disambiguate.
//
//   - Update and Delete return (rowsAffected, err). The repo deliberately
//     does NOT interpret zero rows as an error — what zero means is the
//     caller's call (idempotent delete? conditional update under
//     optimistic locking? batch cleanup with no matches today?). The
//     caller branches on the int64 if it cares.
//
//   - MustGet is a thin wrapper that panics on any error AND on absence
//     (nil result). Reach for it only when the caller has already
//     established that the ID must exist (e.g. derived from a freshly
//     issued foreign key in the same transaction).
type IRepo[T any] interface {
	Add(ctx context.Context, e *T) error
	Update(ctx context.Context, e *T) (int64, error)
	Delete(ctx context.Context, id string) (int64, error)
	FindByID(ctx context.Context, id string) (*T, error)
	MustGet(ctx context.Context, id string) *T
}
