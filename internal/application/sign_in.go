package application

import (
	"context"
	"strings"

	"go-app-template/internal/domain"
)

// SignInCmd verifies a password credential and returns the matching
// Account. It does not issue tokens — session/JWT issuance is intentionally
// out of scope until the project picks an auth scheme.
type SignInCmd struct {
	repo   domain.IAccountRepo
	hasher IPasswordHasher
}

func NewSignInCmd(repo domain.IAccountRepo, hasher IPasswordHasher) *SignInCmd {
	return &SignInCmd{repo: repo, hasher: hasher}
}

// SignInInput is the request payload accepted by SignInCmd.Run.
type SignInInput struct {
	LoginID  string
	Password string
}

// SignInOutput carries the authenticated account. Wrapped in a dedicated
// struct so future additions (session token, last-login timestamp, …)
// extend the return shape without breaking call sites.
type SignInOutput struct {
	Account *domain.Account
}

// Run authenticates a (LoginID, Password) pair. The loginID is matched
// against the email column if it contains '@', otherwise the phone column.
// Every failure mode (unknown id, no password on account, password
// mismatch) returns the same ErrInvalidCredentials sentinel to prevent
// enumeration.
func (c *SignInCmd) Run(ctx context.Context, in SignInInput) (SignInOutput, error) {
	finder := c.repo.FindByPhone
	if strings.Contains(in.LoginID, "@") {
		finder = c.repo.FindByEmail
	}

	acct, err := finder(ctx, in.LoginID)
	if err != nil {
		return SignInOutput{}, wrapRepoErr(err)
	}
	if acct == nil {
		return SignInOutput{}, domain.ErrInvalidCredentials
	}
	if err := acct.Authenticate(in.Password, c.hasher.Compare); err != nil {
		return SignInOutput{}, err
	}
	return SignInOutput{Account: acct}, nil
}
