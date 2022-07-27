package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/service"
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

type AuthHandler struct {
	authSvc service.AuthService
}

func NewAuthHandler(authSvc service.AuthService) *AuthHandler {
	return &AuthHandler{
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
func (h *AuthHandler) Signup(c echo.Context) error {
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
func (h *AuthHandler) Login(c echo.Context) error {
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
func (h *AuthHandler) Logout(c echo.Context) error {
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
func (h *AuthHandler) Refresh(c echo.Context) error {
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
