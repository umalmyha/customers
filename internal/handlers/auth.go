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

type AuthHandler struct {
	authSvc service.AuthService
}

func NewAuthHandler(authSvc service.AuthService) *AuthHandler {
	return &AuthHandler{
		authSvc: authSvc,
	}
}

func (h *AuthHandler) Signup(c echo.Context) error {
	signup := struct {
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required,min=4,max=24"`
	}{}
	if err := c.Bind(&signup); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&signup); err != nil {
		return err
	}

	newUser, err := h.authSvc.Signup(c.Request().Context(), signup.Email, signup.Password)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &struct {
		Id    string `json:"id"`
		Email string `json:"email"`
	}{
		Id:    newUser.Id,
		Email: newUser.Email,
	})
}

func (h *AuthHandler) Login(c echo.Context) error {
	login := struct {
		Email       string `json:"email" validate:"required,email"`
		Password    string `json:"password" validate:"required"`
		Fingerprint string `json:"fingerprint" validate:"required"`
	}{}
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

func (h *AuthHandler) Logout(c echo.Context) error {
	rfrToken := struct {
		RefreshToken string `json:"refreshToken" validate:"required,uuid"`
	}{}
	if err := c.Bind(&rfrToken); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&rfrToken); err != nil {
		return err
	}

	if err := h.authSvc.Logout(c.Request().Context(), rfrToken.RefreshToken); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

func (h *AuthHandler) Refresh(c echo.Context) error {
	refresh := struct {
		Fingerprint  string `json:"fingerprint" validate:"required"`
		RefreshToken string `json:"refreshToken" validate:"required,uuid"`
	}{}
	if err := c.Bind(&refresh); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.Validate(&refresh); err != nil {
		return err
	}

	jwt, rfrToken, err := h.authSvc.Refresh(c.Request().Context(), refresh.RefreshToken, refresh.Fingerprint, time.Now().UTC())
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &session{
		Token:        jwt.Signed,
		ExpiresAt:    jwt.ExpiresAt,
		RefreshToken: rfrToken.Id,
	})
}
