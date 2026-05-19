package domain

// IVisitable marks a domain type that exposes its state through a structured
// visitor protocol rather than direct field access. The type parameter V is
// the aggregate-specific visitor interface (e.g. IAccountVisitor); the
// implementation's Accept walks each field by calling the matching visitor
// method.
//
// Use cases: building DTOs, serialization, partial updates, patch diffs —
// without leaking the aggregate's internal fields.
type IVisitable[V any] interface {
	Accept(visitor V)
}
