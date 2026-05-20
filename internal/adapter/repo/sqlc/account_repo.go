package sqlc

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

	sqlcgen "go-app-template/internal/adapter/repo/sqlc/gen"
	"go-app-template/internal/domain"
)

// pgUniqueViolation is the libpq SQLSTATE for "duplicate key value
// violates unique constraint" — see base_repo.go in the go-jet adapter
// for the same constant; redeclared here to keep the two adapters
// independent (neither imports the other).
const pgUniqueViolation = "23505"

// accountUniqueViolation maps each unique constraint on the account
// table to its domain meaning. All three are credential clashes and map
// to the same sentinel. Lookup misses fall through to the raw error.
var accountUniqueViolation = map[string]error{
	"account_email_key":                     domain.ErrAccountCredentialTaken,
	"account_phone_key":                     domain.ErrAccountCredentialTaken,
	"account_provider_provider_user_id_key": domain.ErrAccountCredentialTaken,
}

// AccountRepo persists domain.Account using sqlc-generated queries against
// PostgreSQL. It is the sqlc counterpart to internal/adapter/repo/jet.AccountRepo
// (which uses go-jet): same domain.IAccountRepo contract, same error
// semantics, different SQL toolchain.
//
// Unlike the go-jet adapter there is no generic baseRepo here — sqlc emits
// concrete, per-query typed methods on *sqlcgen.Queries, so the repo simply
// delegates to them rather than driving a column-list abstraction.
type AccountRepo struct {
	q      *sqlcgen.Queries
	mapper IMapper[domain.Account, sqlcgen.Account]
}

// NewAccountRepo wires a repository over a *sql.DB or *sql.Tx (both satisfy
// sqlcgen.DBTX). The mapper is injected so its own dependencies (Clock,
// IDGen, …) stay out of this constructor's signature.
func NewAccountRepo(db sqlcgen.DBTX, mapper IMapper[domain.Account, sqlcgen.Account]) *AccountRepo {
	return &AccountRepo{
		q:      sqlcgen.New(db),
		mapper: mapper,
	}
}

// Add inserts the aggregate and writes the persisted row back into *e, so
// DB-defaulted columns (created_at/updated_at NOW(), email_verified) are
// reflected in the in-memory instance after one round trip.
func (r *AccountRepo) Add(ctx context.Context, e *domain.Account) error {
	rec := r.mapper.ToRecord(e)
	row, err := r.q.AddAccount(ctx, sqlcgen.AddAccountParams{
		ID:             rec.ID,
		Name:           rec.Name,
		Email:          rec.Email,
		Phone:          rec.Phone,
		AvatarURL:      rec.AvatarURL,
		PasswordHash:   rec.PasswordHash,
		EmailVerified:  rec.EmailVerified,
		Provider:       rec.Provider,
		ProviderUserID: rec.ProviderUserID,
		CreatedAt:      rec.CreatedAt,
		UpdatedAt:      rec.UpdatedAt,
	})
	if err != nil {
		return translateConstraintErr(err)
	}
	*e = *r.mapper.ToDomain(&row)
	return nil
}

// Update returns (rowsAffected, err). Zero rows is not interpreted as an
// error here — the caller decides what zero means in its context.
func (r *AccountRepo) Update(ctx context.Context, e *domain.Account) (int64, error) {
	rec := r.mapper.ToRecord(e)
	n, err := r.q.UpdateAccount(ctx, sqlcgen.UpdateAccountParams{
		ID:             rec.ID,
		Name:           rec.Name,
		Email:          rec.Email,
		Phone:          rec.Phone,
		AvatarURL:      rec.AvatarURL,
		PasswordHash:   rec.PasswordHash,
		EmailVerified:  rec.EmailVerified,
		Provider:       rec.Provider,
		ProviderUserID: rec.ProviderUserID,
		CreatedAt:      rec.CreatedAt,
		UpdatedAt:      rec.UpdatedAt,
	})
	if err != nil {
		return 0, translateConstraintErr(err)
	}
	return n, nil
}

// Delete returns (rowsAffected, err). Same contract as Update.
func (r *AccountRepo) Delete(ctx context.Context, id string) (int64, error) {
	return r.q.DeleteAccount(ctx, id)
}

// FindByID returns (nil, nil) when no row matches.
func (r *AccountRepo) FindByID(ctx context.Context, id string) (*domain.Account, error) {
	row, err := r.q.GetAccount(ctx, id)
	return r.mapRow(row, err)
}

// MustGet panics on any operational error or on absence. Reach for it only
// when the caller has already established that the ID must exist.
func (r *AccountRepo) MustGet(ctx context.Context, id string) *domain.Account {
	e, err := r.FindByID(ctx, id)
	if err != nil {
		panic(err)
	}
	if e == nil {
		panic("MustGet: id " + id + " not found")
	}
	return e
}

func (r *AccountRepo) FindByEmail(ctx context.Context, email string) (*domain.Account, error) {
	row, err := r.q.GetAccountByEmail(ctx, &email)
	return r.mapRow(row, err)
}

func (r *AccountRepo) FindByPhone(ctx context.Context, phone string) (*domain.Account, error) {
	row, err := r.q.GetAccountByPhone(ctx, &phone)
	return r.mapRow(row, err)
}

func (r *AccountRepo) FindByProvider(ctx context.Context, provider, providerUserID string) (*domain.Account, error) {
	row, err := r.q.GetAccountByProvider(ctx, sqlcgen.GetAccountByProviderParams{
		Provider:       &provider,
		ProviderUserID: &providerUserID,
	})
	return r.mapRow(row, err)
}

// mapRow translates a single-row sqlc result into a domain aggregate.
// Absent row → (nil, nil); operational failure → (nil, err); hit →
// (acct, nil).
func (r *AccountRepo) mapRow(row sqlcgen.Account, err error) (*domain.Account, error) {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return r.mapper.ToDomain(&row), nil
}

// translateConstraintErr converts a libpq unique-violation on a known
// account constraint into the matching domain sentinel; anything else
// (including unique violations on other tables, or non-unique errors)
// propagates verbatim.
func translateConstraintErr(err error) error {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != pgUniqueViolation {
		return err
	}
	if mapped, ok := accountUniqueViolation[pqErr.Constraint]; ok {
		return mapped
	}
	return err
}

// Compile-time check that *AccountRepo satisfies the IAccountRepo port.
var _ domain.IAccountRepo = (*AccountRepo)(nil)
