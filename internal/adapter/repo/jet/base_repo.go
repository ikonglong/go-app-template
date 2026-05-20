package jet

import (
	"context"
	"errors"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/lib/pq"
)

// pgUniqueViolation is the libpq SQLSTATE for "duplicate key value violates
// unique constraint". The numeric code is a stable part of the SQL standard
// (class 23 = integrity_constraint_violation, subclass 505 = unique).
const pgUniqueViolation = "23505"

// baseRepo is the generic implementation of domain.IRepo[D] (the
// collection-style Add / Update / Delete / FindByID / MustGet) over a
// single go-jet table whose primary key is a single string column.
// Concrete repositories embed it and add aggregate-specific finders.
//
// D is the domain type (e.g. domain.Account) and R is the corresponding
// go-jet record type (e.g. record.Account). An IMapper translates between
// the two so the domain layer never imports go-jet, and so any
// dependencies the mapping needs (Clock, IDGen, …) live on the mapper
// rather than leaking into the repository constructor.
type baseRepo[D any, R any] struct {
	db qrm.DB

	table       postgres.Table
	allCols     postgres.ColumnList
	mutableCols postgres.ColumnList
	idCol       postgres.ColumnString

	mapper IMapper[D, R]
	idOf   func(*D) string

	// uniqueViolation maps a Postgres unique-constraint name to the domain
	// sentinel that should be returned when *that specific* constraint fires.
	// Lookup misses fall through to propagating the raw libpq error — only
	// constraints the aggregate has explicit semantics for get translated.
	// Nil map disables translation entirely.
	uniqueViolation map[string]error
}

func (r *baseRepo[D, R]) Add(ctx context.Context, e *D) error {
	stmt := r.table.
		INSERT(r.allCols).
		MODEL(r.mapper.ToRecord(e)).
		RETURNING(r.allCols)

	var dest R
	if err := stmt.QueryContext(ctx, r.db, &dest); err != nil {
		return r.translateConstraintErr(err)
	}
	*e = *r.mapper.ToDomain(&dest)
	return nil
}

// Update returns (rowsAffected, err). The repo does NOT interpret zero
// rows as a not-found error — what zero means is the caller's call
// (idempotent path? conditional update under optimistic locking? …).
func (r *baseRepo[D, R]) Update(ctx context.Context, e *D) (int64, error) {
	stmt := r.table.
		UPDATE(r.mutableCols).
		MODEL(r.mapper.ToRecord(e)).
		WHERE(r.idCol.EQ(postgres.String(r.idOf(e))))

	res, err := stmt.ExecContext(ctx, r.db)
	if err != nil {
		return 0, r.translateConstraintErr(err)
	}
	return res.RowsAffected()
}

// Delete returns (rowsAffected, err). Same contract as Update — caller
// decides whether zero rows is an error in their context.
func (r *baseRepo[D, R]) Delete(ctx context.Context, id string) (int64, error) {
	stmt := r.table.
		DELETE().
		WHERE(r.idCol.EQ(postgres.String(id)))

	res, err := stmt.ExecContext(ctx, r.db)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// FindByID returns (nil, nil) when no row matches. The absence of a row
// is a normal lookup outcome; the error channel is reserved for
// operational failures.
func (r *baseRepo[D, R]) FindByID(ctx context.Context, id string) (*D, error) {
	return r.findOne(ctx, r.idCol.EQ(postgres.String(id)))
}

// MustGet fetches the row by primary key and panics on any operational
// error or on absence. Reach for it only when the caller has already
// established that the ID must exist (e.g. derived from a freshly
// issued foreign key in the same transaction).
func (r *baseRepo[D, R]) MustGet(ctx context.Context, id string) *D {
	e, err := r.FindByID(ctx, id)
	if err != nil {
		panic(err)
	}
	if e == nil {
		panic("MustGet: id " + id + " not found")
	}
	return e
}

// findOne is exposed to embedding repositories so they can implement
// aggregate-specific finders without re-implementing row mapping.
// Returns (nil, nil) for no row, (record, nil) on hit, (_, err) for
// operational failure.
func (r *baseRepo[D, R]) findOne(ctx context.Context, where postgres.BoolExpression) (*D, error) {
	stmt := postgres.SELECT(r.allCols).
		FROM(r.table).
		WHERE(where).
		LIMIT(1)

	var dest R
	if err := stmt.QueryContext(ctx, r.db, &dest); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return r.mapper.ToDomain(&dest), nil
}

// translateConstraintErr converts a libpq unique-violation into the
// aggregate's matching domain sentinel when uniqueViolation has an entry
// for the constraint that fired. All other errors (including unique
// violations on constraints not listed in the map) propagate verbatim so
// the caller still sees the raw cause for unexpected failures.
func (r *baseRepo[D, R]) translateConstraintErr(err error) error {
	if r.uniqueViolation == nil {
		return err
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != pgUniqueViolation {
		return err
	}
	if mapped, ok := r.uniqueViolation[pqErr.Constraint]; ok {
		return mapped
	}
	return err
}
