package sqlcdb

// IMapper translates between a domain aggregate type D and its sqlc-generated
// record type R (the struct sqlc emits in package sqlcgen). Implementations
// live next to each aggregate's repository (e.g. AccountMapper) and are the
// natural home for any cross-cutting dependencies that mapping needs — Clock,
// IDGen, value-object factories, password hashers — so those deps don't leak
// into the repository's constructor.
//
// This mirrors the IMapper in the go-jet adapter (internal/adapter/out/db):
// same contract, different record type. The interface is re-declared here
// rather than shared because the two adapters are independent — neither
// should import the other.
type IMapper[D any, R any] interface {
	ToRecord(*D) *R
	ToDomain(*R) *D
}
