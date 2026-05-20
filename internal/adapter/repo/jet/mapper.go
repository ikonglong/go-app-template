package jet

// IMapper translates between a domain aggregate type D and its persistence
// record type R. Implementations live next to each aggregate's repository
// (e.g. AccountMapper) and are the natural home for any cross-cutting
// dependencies that mapping needs — Clock, IDGen, value-object factories,
// password hashers — so those deps don't leak into the repository's
// constructor.
type IMapper[D any, R any] interface {
	ToRecord(*D) *R
	ToDomain(*R) *D
}
