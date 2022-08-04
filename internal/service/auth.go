package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/customers/internal/auth"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/pkg/db/transactor"
)

// AuthService represents auth service behavior
type AuthService interface {
	Signup(context.Context, string, string) (*model.User, error)
	Login(context.Context, string, string, string, time.Time) (*auth.Jwt, *model.RefreshToken, error)
	Logout(context.Context, string) error
	Refresh(context.Context, string, string, time.Time) (*auth.Jwt, *model.RefreshToken, error)
}

type authService struct {
	txtor       transactor.Transactor
	userRps     repository.UserRepository
	rfrTknRps   repository.RefreshTokenRepository
	jwtIssuer   *auth.JwtIssuer
	rfrTokenCfg config.RefreshTokenCfg
}

// NewAuthService builds new authService
func NewAuthService(
	jwtIssuer *auth.JwtIssuer,
	rfrTokenCfg config.RefreshTokenCfg,
	txtor transactor.Transactor,
	userRps repository.UserRepository,
	rfrTknRps repository.RefreshTokenRepository,
) AuthService {
	return &authService{
		jwtIssuer:   jwtIssuer,
		rfrTokenCfg: rfrTokenCfg,
		txtor:       txtor,
		userRps:     userRps,
		rfrTknRps:   rfrTknRps,
	}
}

func (s *authService) Signup(ctx context.Context, email, password string) (*model.User, error) {
	existingUser, err := s.userRps.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if existingUser != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("user with email %s already exist", email))
	}

	hash, err := auth.GeneratePasswordHash(password)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to generate password hash - %v", err))
	}

	u := &model.User{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: hash,
	}

	if err := s.userRps.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *authService) Login(ctx context.Context, email, password, fingerprint string, now time.Time) (jwtToken *auth.Jwt, rfrToken *model.RefreshToken, e error) {
	e = s.txtor.WithinTransaction(ctx, func(ctx context.Context) error {
		user, err := s.userRps.FindByEmail(ctx, email)
		if err != nil {
			return err
		}

		if user == nil {
			return echo.ErrUnauthorized
		}

		err = auth.VerifyPassword(user.PasswordHash, password)
		if err != nil {
			return echo.ErrUnauthorized
		}

		jwtToken, err = s.jwtIssuer.Sign(email, now)
		if err != nil {
			return err
		}

		userTokens, err := s.rfrTknRps.FindTokensByUserID(ctx, user.ID)
		if err != nil {
			return err
		}

		if len(userTokens) >= s.rfrTokenCfg.MaxCount {
			logrus.Infof("max refresh tokens count %d is exceeded for user %s - removing all tokens before generation of new one", s.rfrTokenCfg.MaxCount, user.Email)
			if err := s.rfrTknRps.DeleteByUserID(ctx, user.ID); err != nil {
				return err
			}
		}

		rfrToken = s.refreshToken(user.ID, fingerprint, now)
		if err := s.rfrTknRps.Create(ctx, rfrToken); err != nil {
			return err
		}

		return nil
	})

	return jwtToken, rfrToken, e
}

func (s *authService) Refresh(ctx context.Context, rfrTokenID, fingerprint string, now time.Time) (*auth.Jwt, *model.RefreshToken, error) {
	rfrToken, err := s.rfrTknRps.FindByID(ctx, rfrTokenID)
	if err != nil {
		return nil, nil, err
	}

	if rfrToken == nil {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "invalid refresh token provided")
	}

	err = s.rfrTknRps.DeleteByID(ctx, rfrToken.ID)
	if err != nil {
		return nil, nil, err
	}

	if rfrToken.Fingerprint != fingerprint {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "invalid fingerprint provided")
	}

	if rfrToken.CreatedAt.Add(time.Duration(rfrToken.ExpiresIn) * time.Second).Before(now) {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "refresh token already expired")
	}

	user, err := s.userRps.FindByID(ctx, rfrToken.UserID)
	if err != nil {
		return nil, nil, err
	}

	jwtToken, err := s.jwtIssuer.Sign(user.Email, now)
	if err != nil {
		return nil, nil, err
	}

	newRfrToken := s.refreshToken(user.ID, fingerprint, now)
	if err := s.rfrTknRps.Create(ctx, newRfrToken); err != nil {
		return nil, nil, err
	}

	return jwtToken, newRfrToken, nil
}

func (s *authService) Logout(ctx context.Context, rfrTokenID string) error {
	if err := s.rfrTknRps.DeleteByID(ctx, rfrTokenID); err != nil {
		return err
	}
	return nil
}

func (s *authService) refreshToken(userID, fingerprint string, createdAt time.Time) *model.RefreshToken {
	return &model.RefreshToken{
		ID:          uuid.NewString(),
		UserID:      userID,
		Fingerprint: fingerprint,
		ExpiresIn:   int(s.rfrTokenCfg.TimeToLive.Seconds()),
		CreatedAt:   createdAt,
	}
}
