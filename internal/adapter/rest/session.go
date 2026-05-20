package rest

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/ikonglong/go-apperror"

	"go-app-template/internal/application"
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
	signIn *application.SignInCmd
}

func NewSessionHandler(signIn *application.SignInCmd) *SessionHandler {
	return &SessionHandler{signIn: signIn}
}

func (h *SessionHandler) Register(r route.IRouter) {
	r.POST("/sessions", h.SignIn)
}

type signInReq struct {
	LoginID  string `json:"login_id"`
	Password string `json:"password"`
}

// SignIn implements POST /sessions. The service decides whether login_id
// is an email or a phone (by the presence of '@'); the handler only
// guards against missing fields.
func (h *SessionHandler) SignIn(ctx context.Context, c *app.RequestContext) {
	var req signInReq
	if err := c.BindJSON(&req); err != nil {
		renderError(ctx, c, apperror.NewIllegalInput("session.create", apperror.WithMessage("invalid JSON body"), apperror.WithCause(err)))
		return
	}
	if err := req.validate(); err != nil {
		renderError(ctx, c, err)
		return
	}

	out, err := h.signIn.Run(ctx, application.SignInInput{
		LoginID:  req.LoginID,
		Password: req.Password,
	})
	if err != nil {
		renderError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, buildAccountResp(out.Account))
}

// validate checks structural rules only — credential verification happens
// in the application service.
func (r *signInReq) validate() error {
	if r.LoginID == "" || r.Password == "" {
		return apperror.NewIllegalInput("session.create", apperror.WithMessage("login_id and password are required"))
	}
	return nil
}
