package rest

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/ikonglong/go-apperror"

	"go-app-template/internal/application/account"
	"go-app-template/internal/domain"
)

// AccountHandler exposes the Account-aggregate endpoints. Sign-in lives on
// SessionHandler (POST /sessions) — sign-in is creating a session, not an
// account, so a separate resource is the more RESTful split and also
// avoids using GET for an operation with side effects.
type AccountHandler struct {
	signUp *account.SignUpService
}

func NewAccountHandler(signUp *account.SignUpService) *AccountHandler {
	return &AccountHandler{signUp: signUp}
}

func (h *AccountHandler) Register(r route.IRouter) {
	r.POST("/accounts", h.SignUp)
}

type signUpRequest struct {
	Name     string  `json:"name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Password string  `json:"password"`
}

// SignUp implements POST /accounts. Structural validation happens here at
// the boundary (per CLAUDE.md: "validate user input at the inbound
// adapter"); the application service trusts the inputs it receives.
func (h *AccountHandler) SignUp(ctx context.Context, c *app.RequestContext) {
	var req signUpRequest
	if err := c.BindJSON(&req); err != nil {
		renderError(ctx, c, apperror.NewIllegalInput("account.create", apperror.WithMessage("invalid JSON body"), apperror.WithCause(err)))
		return
	}
	if err := validateSignUp(&req); err != nil {
		renderError(ctx, c, err)
		return
	}

	acct, err := h.signUp.SignUp(ctx, strings.TrimSpace(req.Name), normalizeOptional(req.Email), normalizeOptional(req.Phone), req.Password)
	if err != nil {
		renderError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusCreated, buildAccountResponse(acct))
}

// validateSignUp checks structural rules only — uniqueness lives in the
// application service. Each branch returns a fresh AppError with event
// "account.create" so log aggregations on event survive even when the
// failure aborts before reaching the service.
func validateSignUp(req *signUpRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return apperror.NewIllegalInput("account.create", apperror.WithMessage("name is required"))
	}
	if len(req.Password) < 8 {
		return apperror.NewIllegalInput("account.create", apperror.WithMessage("password must be at least 8 characters"))
	}
	email := normalizeOptional(req.Email)
	phone := normalizeOptional(req.Phone)
	if email == nil && phone == nil {
		return apperror.NewIllegalInput("account.create", apperror.WithMessage("at least one of email or phone is required"))
	}
	if email != nil && !strings.Contains(*email, "@") {
		return apperror.NewIllegalInput("account.create", apperror.WithMessage("email must contain '@'"))
	}
	return nil
}

// normalizeOptional treats whitespace-only strings as absent so a client
// that sends `"phone": ""` doesn't accidentally claim the empty string as
// their phone (which would also fail a real format check downstream).
func normalizeOptional(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// accountResponse is the public JSON shape. password_hash is deliberately
// absent — it never leaves this package — and accountResponseBuilder
// enforces that by overriding only the fields we choose to expose.
type accountResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Email          *string   `json:"email"`
	Phone          *string   `json:"phone"`
	AvatarURL      *string   `json:"avatar_url"`
	EmailVerified  bool      `json:"email_verified"`
	Provider       *string   `json:"provider"`
	ProviderUserID *string   `json:"provider_user_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// accountResponseBuilder visits an Account and collects exposed fields
// into accountResponse. Embedding DefaultAccountVisitor gives every
// unhandled field (currently just PasswordHash) a no-op default, so the
// password hash can never leak; the trade-off is that *new* domain fields
// added to IAccountVisitor will also default to "hidden" until someone
// adds an override here — the right safety direction for an outbound API
// (an accidentally-leaked private field is worse than an
// accidentally-hidden public one, which is visible on the next code
// review).
type accountResponseBuilder struct {
	domain.DefaultAccountVisitor
	rsp accountResponse
}

func (b *accountResponseBuilder) ID(id string)                { b.rsp.ID = id }
func (b *accountResponseBuilder) Name(name string)            { b.rsp.Name = name }
func (b *accountResponseBuilder) Email(email *string)         { b.rsp.Email = email }
func (b *accountResponseBuilder) Phone(phone *string)         { b.rsp.Phone = phone }
func (b *accountResponseBuilder) AvatarURL(url *string)       { b.rsp.AvatarURL = url }
func (b *accountResponseBuilder) EmailVerified(verified bool) { b.rsp.EmailVerified = verified }
func (b *accountResponseBuilder) Provider(provider *string)   { b.rsp.Provider = provider }
func (b *accountResponseBuilder) ProviderUserID(id *string)   { b.rsp.ProviderUserID = id }
func (b *accountResponseBuilder) CreatedAt(t time.Time) { b.rsp.CreatedAt = t }
func (b *accountResponseBuilder) UpdatedAt(t time.Time) { b.rsp.UpdatedAt = t }

func buildAccountResponse(a *domain.Account) *accountResponse {
	b := &accountResponseBuilder{}
	a.Accept(b)
	return &b.rsp
}
