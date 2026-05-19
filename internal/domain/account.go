package domain

import (
	"context"
	"time"

	"github.com/ikonglong/go-apperror"
)

// ErrAccountNotFound is a prebuilt sentinel for callers that need to
// signal "this account should have existed but doesn't". It carries
// apperror.CodeNotFound (maps to HTTP 404 at the wire boundary) and case
// "account_not_found" for log/metric filtering.
//
// NOT returned by IAccountRepo. The repo's finders signal absence with
// (nil, nil) — see IRepo's doc. This sentinel is for **application code
// and handlers** that have decided the absence is exceptional in their
// context (e.g. a GET /accounts/{id} endpoint), so they don't have to
// rebuild the same apperror.NewNotFound(...) at every site.
var ErrAccountNotFound = apperror.NewNotFound(
	"account.lookup",
	apperror.WithMessage("account not found"),
	apperror.WithCase(apperror.NewStrCase("account_not_found")),
)

// ErrAccountCredentialTaken is returned by SignUp when the supplied email or
// phone already belongs to an existing account. Carries case
// "account_credential_taken" so a client/UI can swap the generic "already
// registered" message for a CTA into the password-recovery flow — the
// worked example in error_handling_guide.md §4.6. The Message stays generic
// on purpose; the response should not echo which specific credential clashed
// (helps reduce credential-enumeration noise).
var ErrAccountCredentialTaken = apperror.NewAlreadyExists(
	"account.create",
	apperror.WithMessage("an account with this email or phone already exists"),
	apperror.WithCase(apperror.NewStrCase("account_credential_taken")),
)

// ErrInvalidCredentials is returned by SignIn for any authentication
// failure: unknown login id, password mismatch, or account that has no
// password credential (OAuth-only). The same sentinel is used for all three
// cases on purpose — distinguishing them would let an attacker enumerate
// which emails/phones are registered. No Case is set: no caller is expected
// to branch beyond "auth failed".
var ErrInvalidCredentials = apperror.NewUnauthenticated(
	"session.create",
	apperror.WithMessage("invalid credentials"),
)

// Account is the aggregate root for an authenticated principal. It carries
// either a password credential (passwordHash) or an OAuth credential
// (provider + providerUserID), or both — the schema enforces that at least
// one is present.
//
// All fields are private. Read access goes through the visitor protocol
// (Accept / IAccountVisitor); the only direct getter is ID, which is the
// entity's stable identity. Construction goes through one of two factories:
//   - CreateAccount  — birth of a brand-new aggregate (fresh lifecycle)
//   - RebuildAccount — rehydration of an existing aggregate from storage
type Account struct {
	id            string
	name          string
	email         *string
	phone         *string
	avatarURL     *string
	passwordHash  *string
	emailVerified bool

	provider       *string
	providerUserID *string

	createdAt time.Time
	updatedAt time.Time
}

// CreateAccount creates a fresh Account aggregate — the birth of a new,
// independent lifecycle.
//
// Call this exactly once per aggregate identity in the system's entire
// history: when an account first enters the system (signup, admin invite,
// system bootstrap). "Creating" an Account whose ID already lives in
// storage is a category error — re-creating an id=X aggregate when one
// already exists doesn't produce an independent lifecycle, it produces a
// duplicate. The path for "I already have the bytes, give me the object
// back" is RebuildAccount.
//
// This is the canonical place to enforce invariants on brand-new
// aggregates (currently none; rules go here as they emerge). RebuildAccount
// deliberately skips those checks because storage already cleared them.
//
// Caller supplies "now"; the factory does not read the clock so the
// domain layer stays free of side effects.
func CreateAccount(
	id, name string,
	email, phone, avatarURL, passwordHash *string,
	provider, providerUserID *string,
	now time.Time,
) *Account {
	return &Account{
		id:             id,
		name:           name,
		email:          email,
		phone:          phone,
		avatarURL:      avatarURL,
		passwordHash:   passwordHash,
		emailVerified:  false,
		provider:       provider,
		providerUserID: providerUserID,
		createdAt:      now,
		updatedAt:      now,
	}
}

// RebuildAccount reassembles an Account from a previously persisted state —
// the inverse of writing the aggregate to storage. It is the persistence
// layer's reentry point: AccountMapper.ToDomain calls it to rebuild a
// domain object from a database row.
//
// It skips business-rule validation on purpose. The data being passed in
// already lived in the database, which means it has already cleared every
// invariant the aggregate enforces at write time; re-checking here would
// either be a duplicate of the schema/domain checks or, worse, reject a
// row that the system itself wrote.
//
// Do NOT use this from application or domain code to create a brand-new
// Account. That path belongs to CreateAccount, which is the birth event
// for a fresh lifecycle and the home for invariant validation.
// RebuildAccount is exclusively for "I have all the bytes back, give me
// the object" — repository load, snapshot replay, or a test fixture that
// wants to bypass validation.
func RebuildAccount(
	id, name string,
	email, phone, avatarURL, passwordHash *string,
	emailVerified bool,
	provider, providerUserID *string,
	createdAt, updatedAt time.Time,
) *Account {
	return &Account{
		id:             id,
		name:           name,
		email:          email,
		phone:          phone,
		avatarURL:      avatarURL,
		passwordHash:   passwordHash,
		emailVerified:  emailVerified,
		provider:       provider,
		providerUserID: providerUserID,
		createdAt:      createdAt,
		updatedAt:      updatedAt,
	}
}

// ID returns the entity's stable identity. The other fields are read via
// the visitor (Accept) to keep the surface small.
func (a *Account) ID() string { return a.id }

// Authenticate verifies a plain-text password against the account's stored
// hash by calling the supplied compare function. Returns nil on match,
// ErrInvalidCredentials on any failure (no password set, or compare reports
// a mismatch).
//
// The compare function is passed in (rather than declaring a hasher port
// in the domain) to keep this package free of project-side imports. The
// application layer typically supplies bcrypt.CompareHashAndPassword.
//
// Both legitimate failure modes — accounts created via OAuth (no password)
// and a wrong password on a password account — collapse to the same
// sentinel so callers cannot distinguish them and therefore cannot use
// the endpoint to enumerate which credentials exist.
func (a *Account) Authenticate(plain string, compare func(hash, plain string) error) error {
	if a.passwordHash == nil {
		return ErrInvalidCredentials
	}
	if err := compare(*a.passwordHash, plain); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// Accept walks the aggregate's fields, dispatching each to the visitor.
func (a *Account) Accept(v IAccountVisitor) {
	v.ID(a.id)
	v.Name(a.name)
	v.Email(a.email)
	v.Phone(a.phone)
	v.AvatarURL(a.avatarURL)
	v.PasswordHash(a.passwordHash)
	v.EmailVerified(a.emailVerified)
	v.Provider(a.provider)
	v.ProviderUserID(a.providerUserID)
	v.CreatedAt(a.createdAt)
	v.UpdatedAt(a.updatedAt)
}

// Compile-time check that *Account satisfies IVisitable[IAccountVisitor].
var _ IVisitable[IAccountVisitor] = (*Account)(nil)

// IAccountVisitor is the per-field visitor for Account. Each method
// receives one field value; pointer-typed parameters preserve nullability.
// Implementations decide what to do with each call (accumulate into a DTO,
// write JSON, diff against another source, etc.).
type IAccountVisitor interface {
	ID(id string)
	Name(name string)
	Email(email *string)
	Phone(phone *string)
	AvatarURL(url *string)
	PasswordHash(hash *string)
	EmailVerified(verified bool)
	Provider(provider *string)
	ProviderUserID(id *string)
	CreatedAt(t time.Time)
	UpdatedAt(t time.Time)
}

// DefaultAccountVisitor is a no-op base implementation of IAccountVisitor.
// Embed it in your visitor type to avoid implementing methods for fields
// you don't care about — only the methods you do override take effect, the
// rest fall through to the no-op defaults. Typical use cases:
//
//   - DTO builders that only need a subset of fields
//   - Redaction visitors that touch only sensitive columns
//   - Partial JSON / log output
//   - Patch / diff detectors that watch a few specific fields
//
// Example: a visitor that only collects identity fields.
//
//	type identityVisitor struct {
//	    DefaultAccountVisitor          // no-op for everything else
//	    id, name string
//	}
//
//	func (v *identityVisitor) ID(id string)     { v.id = id }
//	func (v *identityVisitor) Name(name string) { v.name = name }
//
// Trade-off: embedding the default opts out of the compiler's "every field
// must be handled" check. New fields added to IAccountVisitor will silently
// fall through the no-op defaults instead of failing to compile. Don't
// embed it where dropping a future field would be a bug — for example, a
// full-fidelity persistence mapper should implement every method directly
// (see the AccountMapper discussion).
type DefaultAccountVisitor struct{}

func (DefaultAccountVisitor) ID(string)              {}
func (DefaultAccountVisitor) Name(string)            {}
func (DefaultAccountVisitor) Email(*string)          {}
func (DefaultAccountVisitor) Phone(*string)          {}
func (DefaultAccountVisitor) AvatarURL(*string)      {}
func (DefaultAccountVisitor) PasswordHash(*string)   {}
func (DefaultAccountVisitor) EmailVerified(bool)     {}
func (DefaultAccountVisitor) Provider(*string)       {}
func (DefaultAccountVisitor) ProviderUserID(*string) {}
func (DefaultAccountVisitor) CreatedAt(time.Time)    {}
func (DefaultAccountVisitor) UpdatedAt(time.Time)    {}

var _ IAccountVisitor = DefaultAccountVisitor{}

// IAccountRepo is the outbound port for persisting accounts: the generic
// collection-style operations (Add / Update / Delete / FindByID / MustGet)
// plus the unique-key lookups specific to the account aggregate.
//
// The aggregate-specific finders below follow the same contract as
// IRepo's FindByID: (nil, nil) on absence, (acct, nil) on hit, (_, err)
// only for operational failures.
type IAccountRepo interface {
	IRepo[Account]

	FindByEmail(ctx context.Context, email string) (*Account, error)
	FindByPhone(ctx context.Context, phone string) (*Account, error)
	FindByProvider(ctx context.Context, provider, providerUserID string) (*Account, error)
}
