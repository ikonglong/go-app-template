package application

import (
	"errors"

	"github.com/ikonglong/go-apperror"

	"go-app-template/internal/domain"
)

// wrapRepoErr converts a RemoteError from a persistence adapter into the
// appropriate AppError from the application's outward-contract
// perspective. Non-RemoteErrors pass through unchanged.
//
// This is the Core's wrap step: RemoteErrors from the "app calls remote
// service" context are re-judged from the "app Client calls app API"
// perspective — the responsibility assignment and code that were correct
// for the remote call may need adjustment for the application's own
// outward contract.
func wrapRepoErr(err error) error {
	var re *apperror.RemoteError
	if !errors.As(err, &re) {
		return err
	}
	switch re.Code() {
	case apperror.CodeAlreadyExists:
		// A unique-constraint violation on a credential column maps to the
		// account-credential-taken business error. The sentinel is returned
		// directly (no cause wrapping) because the diagnostic value of the
		// underlying pq.Error for a constraint violation is minimal — the
		// sentinel's Code + Case + Message already pinpoints the failure.
		return domain.ErrAccountCredentialTaken
	case apperror.CodeUnavailable:
		return apperror.NewUnavailable("account.create",
			apperror.WithMessage("database unavailable"),
			apperror.WithCause(re))
	default:
		return apperror.NewInternal("account.create",
			apperror.WithMessage("database operation failed"),
			apperror.WithCause(re))
	}
}
