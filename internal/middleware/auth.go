package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/domain/auth"
	"strings"
)

func Authorize(validator *auth.JwtValidator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHdr := c.Request().Header.Get("Authorization")
			hdrSplit := strings.Split(authHdr, " ")
			if len(hdrSplit) != 2 {
				return echo.ErrUnauthorized
			}

			if _, err := validator.Verify(hdrSplit[1]); err != nil {
				return err
			}

			return next(c)
		}
	}
}
