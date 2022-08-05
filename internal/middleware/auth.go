package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/auth"
)

const splitAuthHeaderPartsCount = 2

// Authorize is middleware function for validating Authorization JWT header
func Authorize(validator *auth.JwtValidator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHdr := c.Request().Header.Get("Authorization")
			hdrSplit := strings.Split(authHdr, " ")
			if len(hdrSplit) != splitAuthHeaderPartsCount {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid Authorization header format")
			}

			if _, err := validator.Verify(hdrSplit[1]); err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf("token verification failed - %v", err))
			}

			return next(c)
		}
	}
}
