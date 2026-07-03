package application

import (
	"context"

	"go-app-template/internal/common/clock"
	"go-app-template/internal/common/idgen"
	"go-app-template/internal/domain"
)

// SignUpCmd creates a fresh Account from password-credential signup.
// OAuth signup is a separate use case and out of scope here.
//
// Concurrent-uniqueness gate: the existence checks below race against
// concurrent inserts. The database UNIQUE constraints on email and phone
// remain the authoritative backstop. When a concurrent writer wins the
// race, the persistence adapter translates the unique-violation into a
// RemoteError(AlreadyExists); wrapRepoErr below wraps that into
// ErrAccountCredentialTaken — the same sentinel assertCredentialAvailable
// returns for a straight finder hit.
type SignUpCmd struct {
	repo   domain.IAccountRepo
	hasher IPasswordHasher
	clock  clock.IClock
	idGen  idgen.IIDGen
}

func NewSignUpCmd(repo domain.IAccountRepo, hasher IPasswordHasher, clk clock.IClock, idGen idgen.IIDGen) *SignUpCmd {
	return &SignUpCmd{repo: repo, hasher: hasher, clock: clk, idGen: idGen}
}

// SignUpInput is the request payload accepted by SignUpCmd.Run.
type SignUpInput struct {
	Name     string
	Email    *string
	Phone    *string
	Password string
}

// SignUpOutput carries the newly created account. Wrapped in a dedicated
// struct so future additions (welcome-email job id, default workspace, …)
// extend the return shape without breaking call sites.
type SignUpOutput struct {
	Account *domain.Account
}

// Run creates a new Account. Caller (REST handler) is responsible for
// structural validation (non-empty name/password, email format if present,
// at-least-one of email/phone, …); this method trusts those guarantees and
// only handles business-level rules (credential uniqueness, hashing).
func (c *SignUpCmd) Run(ctx context.Context, in SignUpInput) (SignUpOutput, error) {
	if err := c.assertCredentialAvailable(ctx, in.Email, in.Phone); err != nil {
		return SignUpOutput{}, err
	}

	hash, err := c.hasher.Hash(in.Password)
	if err != nil {
		return SignUpOutput{}, err
	}

	acct := domain.CreateAccount(
		c.idGen.Next(),
		in.Name,
		in.Email, in.Phone,
		nil, // avatarURL
		&hash,
		nil, nil, // provider, providerUserID — password signup path
		c.clock.Now(),
	)
	if err := c.repo.Add(ctx, acct); err != nil {
		return SignUpOutput{}, wrapRepoErr(err)
	}
	return SignUpOutput{Account: acct}, nil
}

// assertCredentialAvailable returns ErrAccountCredentialTaken if either
// the email or the phone already maps to an existing account. The generic
// sentinel deliberately does not reveal *which* credential clashed —
// distinguishing them would let an attacker enumerate registered emails
// vs phones.
func (c *SignUpCmd) assertCredentialAvailable(ctx context.Context, email, phone *string) error {
	if email != nil {
		if err := c.assertUnused(c.repo.FindByEmail(ctx, *email)); err != nil {
			return err
		}
	}
	if phone != nil {
		if err := c.assertUnused(c.repo.FindByPhone(ctx, *phone)); err != nil {
			return err
		}
	}
	return nil
}

// assertUnused turns a finder result into either nil (credential is free)
// or ErrAccountCredentialTaken (someone already owns it). The finder
// contract is (nil, nil) for absence — nil account means "free", any
// non-nil account means "taken".
//
// Operational errors from the finder are wrapped via wrapRepoErr so
// RemoteErrors from the persistence adapter don't reach the Interfaces
// layer unwrapped.
func (c *SignUpCmd) assertUnused(acct *domain.Account, err error) error {
	if err != nil {
		return wrapRepoErr(err)
	}
	if acct != nil {
		return domain.ErrAccountCredentialTaken
	}
	return nil
}
