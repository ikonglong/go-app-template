package account

import (
	"context"
	"strings"

	"go-app-template/internal/domain"
)

// SignInService verifies a password credential and returns the matching
// Account. It does not issue tokens — session/JWT issuance is intentionally
// out of scope until the project picks an auth scheme.
type SignInService struct {
	repo   domain.IAccountRepo
	hasher IPasswordHasher
}

func NewSignInService(repo domain.IAccountRepo, hasher IPasswordHasher) *SignInService {
	return &SignInService{repo: repo, hasher: hasher}
}

// SignIn authenticates a (loginID, password) pair. The loginID is matched
// against the email column if it contains '@', otherwise the phone column.
// Every failure mode (unknown id, no password on account, password
// mismatch) returns the same ErrInvalidCredentials sentinel to prevent
// enumeration.
func (s *SignInService) SignIn(ctx context.Context, loginID, password string) (*domain.Account, error) {
	finder := s.repo.FindByPhone
	if strings.Contains(loginID, "@") {
		finder = s.repo.FindByEmail
	}

	acct, err := finder(ctx, loginID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, domain.ErrInvalidCredentials
	}
	if err := acct.Authenticate(password, s.hasher.Compare); err != nil {
		return nil, err
	}
	return acct, nil
}
