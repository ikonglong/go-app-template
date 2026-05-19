package account

import (
	"context"

	"go-app-template/internal/domain"
)

// SignUpService creates a fresh Account from password-credential signup.
// OAuth signup is a separate use case and out of scope here.
//
// Concurrent-uniqueness gap: the existence checks below race against
// concurrent inserts. The database UNIQUE constraints on email and phone
// remain the authoritative guard, but the resulting libpq error is not yet
// translated to ErrAccountCredentialTaken in the persistence adapter —
// that translation is a planned follow-up. Until then the rare losing
// writer surfaces a generic 500. See repo CLAUDE.md error_handling_guide
// for the planned translation point.
type SignUpService struct {
	repo   domain.IAccountRepo
	hasher IPasswordHasher
	clock  IClock
	idGen  IIDGen
}

func NewSignUpService(repo domain.IAccountRepo, hasher IPasswordHasher, clock IClock, idGen IIDGen) *SignUpService {
	return &SignUpService{repo: repo, hasher: hasher, clock: clock, idGen: idGen}
}

// SignUp creates a new Account. Caller (REST handler) is responsible for
// structural validation (non-empty name/password, email format if present,
// at-least-one of email/phone, …); this method trusts those guarantees and
// only handles business-level rules (credential uniqueness, hashing).
func (s *SignUpService) SignUp(ctx context.Context, name string, email, phone *string, password string) (*domain.Account, error) {
	if err := s.assertCredentialAvailable(ctx, email, phone); err != nil {
		return nil, err
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, err
	}

	acct := domain.CreateAccount(
		s.idGen.Next(),
		name,
		email, phone,
		nil, // avatarURL
		&hash,
		nil, nil, // provider, providerUserID — password signup path
		s.clock.Now(),
	)
	if err := s.repo.Add(ctx, acct); err != nil {
		return nil, err
	}
	return acct, nil
}

// assertCredentialAvailable returns ErrAccountCredentialTaken if either
// the email or the phone already maps to an existing account. The generic
// sentinel deliberately does not reveal *which* credential clashed —
// distinguishing them would let an attacker enumerate registered emails
// vs phones.
func (s *SignUpService) assertCredentialAvailable(ctx context.Context, email, phone *string) error {
	if email != nil {
		if err := s.assertUnused(s.repo.FindByEmail(ctx, *email)); err != nil {
			return err
		}
	}
	if phone != nil {
		if err := s.assertUnused(s.repo.FindByPhone(ctx, *phone)); err != nil {
			return err
		}
	}
	return nil
}

// assertUnused turns a finder result into either nil (credential is free)
// or ErrAccountCredentialTaken (someone already owns it). The finder
// contract is (nil, nil) for absence — nil account means "free", any
// non-nil account means "taken".
func (s *SignUpService) assertUnused(acct *domain.Account, err error) error {
	if err != nil {
		return err
	}
	if acct != nil {
		return domain.ErrAccountCredentialTaken
	}
	return nil
}
