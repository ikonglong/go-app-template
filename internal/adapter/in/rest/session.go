package rest

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/ikonglong/go-apperror"

	"go-app-template/internal/application/account"
)

// SessionHandler exposes session creation (sign-in). It is intentionally
// separate from AccountHandler: signing in produces a session, not a new
// account, and giving sessions their own resource keeps the verb (POST)
// matching the side effect (a write).
//
// No session token is issued today — the response is just the
// authenticated Account JSON. Token issuance is a planned follow-up
// (decide JWT vs opaque session, signing key, refresh strategy).
type SessionHandler struct {
	signIn *account.SignInService
}

func NewSessionHandler(signIn *account.SignInService) *SessionHandler {
	return &SessionHandler{signIn: signIn}
}

func (h *SessionHandler) Register(r route.IRouter) {
	r.POST("/sessions", h.SignIn)
}

type signInRequest struct {
	LoginID  string `json:"login_id"`
	Password string `json:"password"`
}

// SignIn implements POST /sessions. The service decides whether login_id
// is an email or a phone (by the presence of '@'); the handler only
// guards against missing fields.
func (h *SessionHandler) SignIn(ctx context.Context, c *app.RequestContext) {
	var req signInRequest
	if err := c.BindJSON(&req); err != nil {
		renderError(ctx, c, apperror.NewIllegalInput("session.create", apperror.WithMessage("invalid JSON body"), apperror.WithCause(err)))
		return
	}
	if req.LoginID == "" || req.Password == "" {
		renderError(ctx, c, apperror.NewIllegalInput("session.create", apperror.WithMessage("login_id and password are required")))
		return
	}

	acct, err := h.signIn.SignIn(ctx, req.LoginID, req.Password)
	if err != nil {
		renderError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, buildAccountResponse(acct))
}
