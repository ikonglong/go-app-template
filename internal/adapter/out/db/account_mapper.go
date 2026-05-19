package db

import (
	"time"

	"go-app-template/internal/adapter/out/db/gen/test/public/record"
	"go-app-template/internal/domain"
)

// AccountMapper translates domain.Account ↔ record.Account. It is the home
// for any cross-cutting dependencies the mapping needs — add fields on the
// struct (Clock, IDGen, hasher, etc.) and wire them in NewAccountMapper —
// without touching AccountRepo's constructor.
type AccountMapper struct{}

func NewAccountMapper() *AccountMapper {
	return &AccountMapper{}
}

// ToRecord drives Account.Accept with a recordBuilder. The visitor is the
// only way out — Account's fields are private, so direct reads aren't an
// option here.
func (*AccountMapper) ToRecord(a *domain.Account) *record.Account {
	b := &recordBuilder{}
	a.Accept(b)
	return &b.rec
}

func (*AccountMapper) ToDomain(r *record.Account) *domain.Account {
	return domain.RebuildAccount(
		r.ID,
		r.Name,
		r.Email,
		r.Phone,
		r.AvatarURL,
		r.PasswordHash,
		r.EmailVerified,
		r.Provider,
		r.ProviderUserID,
		r.CreatedAt,
		r.UpdatedAt,
	)
}

// recordBuilder is a per-call IAccountVisitor that accumulates an Account's
// fields into a record.Account. It deliberately implements every visitor
// method directly — no DefaultAccountVisitor embedding — so adding a field
// to IAccountVisitor breaks compilation here and forces the mapper to be
// updated in lockstep.
type recordBuilder struct {
	rec record.Account
}

func (b *recordBuilder) ID(id string)               { b.rec.ID = id }
func (b *recordBuilder) Name(name string)           { b.rec.Name = name }
func (b *recordBuilder) Email(email *string)        { b.rec.Email = email }
func (b *recordBuilder) Phone(phone *string)        { b.rec.Phone = phone }
func (b *recordBuilder) AvatarURL(url *string)      { b.rec.AvatarURL = url }
func (b *recordBuilder) PasswordHash(hash *string)  { b.rec.PasswordHash = hash }
func (b *recordBuilder) EmailVerified(verified bool) {
	b.rec.EmailVerified = verified
}
func (b *recordBuilder) Provider(provider *string)  { b.rec.Provider = provider }
func (b *recordBuilder) ProviderUserID(id *string)  { b.rec.ProviderUserID = id }
func (b *recordBuilder) CreatedAt(t time.Time)      { b.rec.CreatedAt = t }
func (b *recordBuilder) UpdatedAt(t time.Time)      { b.rec.UpdatedAt = t }

var (
	_ IMapper[domain.Account, record.Account] = (*AccountMapper)(nil)
	_ domain.IAccountVisitor                  = (*recordBuilder)(nil)
)
