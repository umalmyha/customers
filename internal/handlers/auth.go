package handlers

import (
	"context"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/proto"
	"github.com/umalmyha/customers/internal/service"
	"google.golang.org/protobuf/types/known/emptypb"
	"net/http"
	"time"
)

type session struct {
	Token        string `json:"accessToken"`
	ExpiresAt    int64  `json:"expiresAt"`
	RefreshToken string `json:"refreshToken"`
}

type signup struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=4,max=24"`
}

type logout struct {
	RefreshToken string `json:"refreshToken" validate:"required,uuid"`
}

type newUser struct {
	Id    string `json:"id"`
	Email string `json:"email"`
}

type login struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required"`
	Fingerprint string `json:"fingerprint" validate:"required"`
}

type refresh struct {
	Fingerprint  string `json:"fingerprint" validate:"required"`
	RefreshToken string `json:"refreshToken" validate:"required,uuid"`
}

type authHttpHandler struct {
	authSvc service.AuthService
}

func NewAuthHttpHandler(authSvc service.AuthService) *authHttpHandler {
	return &authHttpHandler{
		authSvc: authSvc,
	}
}

// Signup godoc
// @Summary     Signup new account
// @Description Register new account based on provided credentials
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       signup body	    signup true "New user data"
// @Success     200    {object} newUser
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/auth/signup [post]
func (h *authHttpHandler) Signup(c echo.Context) error {
	var su signup
	if err := c.Bind(&su); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&su); err != nil {
		return err
	}

	nu, err := h.authSvc.Signup(c.Request().Context(), su.Email, su.Password)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &newUser{
		Id:    nu.Id,
		Email: nu.Email,
	})
}

// Login godoc
// @Summary     Login user
// @Description Verifies provided credentials, sign jwt and refresh token
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       login  body	    login true "User credentials"
// @Success     200    {object} session
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/auth/login [post]
func (h *authHttpHandler) Login(c echo.Context) error {
	var login login
	if err := c.Bind(&login); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&login); err != nil {
		return err
	}

	jwt, rfrToken, err := h.authSvc.Login(c.Request().Context(), login.Email, login.Password, login.Fingerprint, time.Now().UTC())
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &session{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.Id,
	})
}

// Logout godoc
// @Summary     Logout user
// @Description Remove any user-related session data
// @Tags        auth
// @Accept      json
// @Param       logout body	    logout true "Refresh token id"
// @Success     200    "Successful status code"
// @Failure     400    {object} echo.HTTPError
// @Failure     500    {object} echo.HTTPError
// @Router      /api/auth/logout [post]
func (h *authHttpHandler) Logout(c echo.Context) error {
	var logout logout
	if err := c.Bind(&logout); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&logout); err != nil {
		return err
	}

	if err := h.authSvc.Logout(c.Request().Context(), logout.RefreshToken); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// Refresh godoc
// @Summary     Refresh jwt
// @Description Sign new jwt and refresh token
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       refresh body	 refresh true "Fingerprint and refresh token id"
// @Success     200     {object} session
// @Failure     400     {object} echo.HTTPError
// @Failure     500     {object} echo.HTTPError
// @Router      /api/auth/refresh [post]
func (h *authHttpHandler) Refresh(c echo.Context) error {
	var r refresh
	if err := c.Bind(&r); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&r); err != nil {
		return err
	}

	jwt, rfrToken, err := h.authSvc.Refresh(c.Request().Context(), r.RefreshToken, r.Fingerprint, time.Now().UTC())
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &session{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.Id,
	})
}

type authGrpcHandler struct {
	proto.UnimplementedAuthServiceServer
	authSvc service.AuthService
}

func NewAuthGrpcHandler(authSvc service.AuthService) *authGrpcHandler {
	return &authGrpcHandler{
		UnimplementedAuthServiceServer: proto.UnimplementedAuthServiceServer{},
		authSvc:                        authSvc,
	}
}

func (h *authGrpcHandler) Signup(ctx context.Context, req *proto.SignupRequest) (*proto.NewUserResponse, error) {
	u, err := h.authSvc.Signup(ctx, req.Email, req.Password)
	if err != nil {
		return nil, err
	}

	return &proto.NewUserResponse{
		Id:    u.Id,
		Email: u.Email,
	}, nil
}

func (h *authGrpcHandler) Login(ctx context.Context, req *proto.LoginRequest) (*proto.SessionResponse, error) {
	jwt, rfrToken, err := h.authSvc.Login(ctx, req.Email, req.Password, req.Fingerprint, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	return &proto.SessionResponse{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.Id,
	}, nil
}

func (h *authGrpcHandler) Logout(ctx context.Context, req *proto.LogoutRequest) (*emptypb.Empty, error) {
	if err := h.authSvc.Logout(ctx, req.RefreshToken); err != nil {
		// TODO: Think of error handling
		return nil, err
	}
	return nil, nil
}

func (h *authGrpcHandler) Refresh(ctx context.Context, req *proto.RefreshRequest) (*proto.SessionResponse, error) {
	jwt, rfrToken, err := h.authSvc.Refresh(ctx, req.RefreshToken, req.Fingerprint, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	return &proto.SessionResponse{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.Id,
	}, nil
}
