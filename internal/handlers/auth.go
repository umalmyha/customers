package handlers

import (
	"context"
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/domain/auth"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"net/http"
	"time"
)

type Identification struct {
	Fingerprint string `json:"fingerprint"`
}

type AccessToken struct {
	Token     string `json:"accessToken"`
	ExpiresAt int64  `json:"expiresAt"`
}

type AuthCfg struct {
	Https              bool
	RefreshTokenCookie string
}

type AuthHandler struct {
	trx     transactor.Transactor
	authSrv service.AuthService
	authCfg AuthCfg
}

func NewAuthHandler(trx transactor.Transactor, authSrv service.AuthService, authCfg AuthCfg) *AuthHandler {
	return &AuthHandler{
		trx:     trx,
		authSrv: authSrv,
		authCfg: authCfg,
	}
}

func (h *AuthHandler) Signup(c echo.Context) error {
	su := struct {
		Email           string `json:"email"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirmPassword"`
	}{}

	if err := c.Bind(&su); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	newUser, err := h.authSrv.Signup(c.Request().Context(), auth.Signup(su))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, newUser)
}

func (h *AuthHandler) Login(c echo.Context) error {
	username, password, ok := c.Request().BasicAuth()
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to get credentials, please use basic auth")
	}

	var ident Identification
	if err := c.Bind(&ident); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return h.trx.WithinTransaction(c.Request().Context(), func(ctx context.Context) error {
		jwt, refresh, err := h.authSrv.Login(ctx, auth.Login{
			Email:       username,
			Password:    password,
			Fingerprint: ident.Fingerprint,
			At:          time.Now().UTC(),
		})
		if err != nil {
			return err
		}

		c.SetCookie(h.refreshTokenCookie(refresh.Id, refresh.ExpiresIn))

		return c.JSON(http.StatusOK, &AccessToken{
			Token:     jwt.Signed,
			ExpiresAt: jwt.ExpiresAt,
		})
	})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	tknCookie, err := c.Cookie(h.authCfg.RefreshTokenCookie)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "refresh token cookie is missing - you are not logged in")
	}

	if err := h.authSrv.Logout(c.Request().Context(), tknCookie.Value); err != nil {
		return err
	}

	tknCookie.MaxAge = -1
	c.SetCookie(tknCookie)

	return c.NoContent(http.StatusOK)
}

func (h *AuthHandler) Refresh(c echo.Context) error {
	tknCookie, err := c.Cookie(h.authCfg.RefreshTokenCookie)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "refresh token cookie is missing - you are not logged in")
	}

	var ident Identification
	if err := c.Bind(&ident); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return h.trx.WithinTransaction(c.Request().Context(), func(ctx context.Context) error {
		jwt, refresh, err := h.authSrv.Refresh(ctx, auth.Refresh{
			Token:       tknCookie.Value,
			Fingerprint: ident.Fingerprint,
			At:          time.Now().UTC(),
		})
		if err != nil {
			if errors.Is(err, auth.ErrRefreshTokenExpired) || errors.Is(err, auth.ErrInvalidFingerprint) {
				return c.JSON(http.StatusBadRequest, err.Error())
			}
			return err
		}

		c.SetCookie(h.refreshTokenCookie(refresh.Id, refresh.ExpiresIn))

		return c.JSON(http.StatusOK, &AccessToken{
			Token:     jwt.Signed,
			ExpiresAt: jwt.ExpiresAt,
		})
	})
}

func (h *AuthHandler) refreshTokenCookie(tknId string, expiresIn int) *http.Cookie {
	return &http.Cookie{
		Name:     h.authCfg.RefreshTokenCookie,
		Value:    tknId,
		Path:     "/api/auth",
		MaxAge:   expiresIn,
		HttpOnly: true,
		Secure:   h.authCfg.Https,
	}
}
