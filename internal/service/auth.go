package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/model/auth"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"net/http"
	"time"
)

type AuthService interface {
	Signup(context.Context, string, string) (*auth.User, error)
	Login(context.Context, string, string, string, time.Time) (*auth.Jwt, *auth.RefreshToken, error)
	Logout(context.Context, string) error
	Refresh(context.Context, string, string, time.Time) (*auth.Jwt, *auth.RefreshToken, error)
}

type authService struct {
	transactor  transactor.Transactor
	userRps     repository.UserRepository
	rfrTknRps   repository.RefreshTokenRepository
	jwtIssuer   *auth.JwtIssuer
	rfrTokenCfg config.RefreshTokenCfg
}

func NewAuthService(
	jwtIssuer *auth.JwtIssuer,
	rfrTokenCfg config.RefreshTokenCfg,
	transactor transactor.Transactor,
	userRps repository.UserRepository,
	rfrTknRps repository.RefreshTokenRepository,
) AuthService {
	return &authService{jwtIssuer: jwtIssuer, rfrTokenCfg: rfrTokenCfg, transactor: transactor, userRps: userRps, rfrTknRps: rfrTknRps}
}

func (s *authService) Signup(ctx context.Context, email string, password string) (*auth.User, error) {
	// TODO: Additional validations - Step 7
	hash, err := auth.GeneratePasswordHash(password)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to generate password hash - %v", err))
	}

	u := &auth.User{
		Id:           uuid.NewString(),
		Email:        email,
		PasswordHash: hash,
	}

	if err := s.userRps.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *authService) Login(ctx context.Context, email string, password string, fingerprint string, now time.Time) (jwtToken *auth.Jwt, rfrToken *auth.RefreshToken, err error) {
	err = s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		user, err := s.userRps.FindByEmail(ctx, email)
		if err != nil {
			return err
		}

		if user == nil {
			return echo.ErrUnauthorized
		}

		if err := user.VerifyPassword(password); err != nil {
			return echo.ErrUnauthorized
		}

		jwtToken, err = s.jwtIssuer.Sign(email, now)
		if err != nil {
			return err
		}

		userTokens, err := s.rfrTknRps.FindTokensByUserId(ctx, user.Id)
		if err != nil {
			return err
		}

		if len(userTokens) >= s.rfrTokenCfg.MaxCount {
			if err := s.rfrTknRps.DeleteByUserId(ctx, user.Id); err != nil {
				return err
			}
		}

		rfrToken = s.refreshToken(user.Id, fingerprint, now)
		if err := s.rfrTknRps.Create(ctx, rfrToken); err != nil {
			return err
		}

		return nil
	})

	return jwtToken, rfrToken, err
}

func (s *authService) Refresh(ctx context.Context, rfrTokenId string, fingerprint string, now time.Time) (*auth.Jwt, *auth.RefreshToken, error) {
	rfrToken, err := s.rfrTknRps.FindById(ctx, rfrTokenId)
	if err != nil {
		return nil, nil, err
	}

	if rfrToken == nil {
		return nil, nil, errors.New("invalid refresh token provided")
	}

	if err := s.rfrTknRps.DeleteById(ctx, rfrToken.Id); err != nil {
		return nil, nil, err
	}

	if err := rfrToken.Verify(fingerprint, now); err != nil {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	user, err := s.userRps.FindById(ctx, rfrToken.UserId)
	if err != nil {
		return nil, nil, err
	}

	jwtToken, err := s.jwtIssuer.Sign(user.Email, now)
	if err != nil {
		return nil, nil, err
	}

	newRfrToken := s.refreshToken(user.Id, fingerprint, now)
	if err := s.rfrTknRps.Create(ctx, newRfrToken); err != nil {
		return nil, nil, err
	}

	return jwtToken, newRfrToken, nil
}

func (s *authService) Logout(ctx context.Context, rfrTokenId string) error {
	if err := s.rfrTknRps.DeleteById(ctx, rfrTokenId); err != nil {
		return err
	}
	return nil
}

func (s *authService) refreshToken(userId string, fingerprint string, createdAt time.Time) *auth.RefreshToken {
	return &auth.RefreshToken{
		Id:          uuid.NewString(),
		UserId:      userId,
		Fingerprint: fingerprint,
		ExpiresIn:   int(s.rfrTokenCfg.TimeToLive.Seconds()),
		CreatedAt:   createdAt,
	}
}
