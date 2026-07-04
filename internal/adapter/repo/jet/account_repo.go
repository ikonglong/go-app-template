package jet

import (
	"context"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/ikonglong/go-apperror"

	"go-app-template/internal/adapter/repo/jet/gen/record"
	"go-app-template/internal/adapter/repo/jet/gen/table"
	"go-app-template/internal/domain"
)

// AccountRepo persists domain.Account using go-jet against PostgreSQL. The
// generic collection-style methods (Add / Update / Delete / FindByID /
// MustGet) come from the embedded baseRepo; only account-specific finders
// live here.
type AccountRepo struct {
	*baseRepo[domain.Account, record.Account]
}

// NewAccountRepo wires a repository over a *sql.DB or *sql.Tx. The mapper is
// injected so its own dependencies (Clock, IDGen, …) stay out of this
// constructor's signature.
func NewAccountRepo(db qrm.DB, mapper IMapper[domain.Account, record.Account]) *AccountRepo {
	return &AccountRepo{
		baseRepo: &baseRepo[domain.Account, record.Account]{
			db:          db,
			table:       table.Account,
			allCols:     table.Account.AllColumns,
			mutableCols: table.Account.MutableColumns,
			idCol:       table.Account.ID,
			mapper:      mapper,
			idOf:        func(a *domain.Account) string { return a.ID() },
				eventPrefix: "postgres.account",
			// All three uniques carry the same domain meaning: the
			// supplied credential is already registered. The application
			// pre-checks via FindByEmail / FindByPhone, but a concurrent
			// writer winning the race lands here — that's the backstop.
			uniqueViolation: map[string]apperror.Code{
				"account_email_key":                     apperror.CodeAlreadyExists,
				"account_phone_key":                     apperror.CodeAlreadyExists,
				"account_provider_provider_user_id_key": apperror.CodeAlreadyExists,
			},
		},
	}
}

func (r *AccountRepo) FindByEmail(ctx context.Context, email string) (*domain.Account, error) {
	return r.findOne(ctx, table.Account.Email.EQ(postgres.String(email)))
}

func (r *AccountRepo) FindByPhone(ctx context.Context, phone string) (*domain.Account, error) {
	return r.findOne(ctx, table.Account.Phone.EQ(postgres.String(phone)))
}

func (r *AccountRepo) FindByProvider(ctx context.Context, provider, providerUserID string) (*domain.Account, error) {
	return r.findOne(ctx,
		table.Account.Provider.EQ(postgres.String(provider)).
			AND(table.Account.ProviderUserID.EQ(postgres.String(providerUserID))),
	)
}

// Compile-time check that *AccountRepo satisfies the IAccountRepo port.
var _ domain.IAccountRepo = (*AccountRepo)(nil)
