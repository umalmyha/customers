package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
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
	logger      logrus.FieldLogger
	jwtIssuer   *auth.JwtIssuer
	rfrTokenCfg config.RefreshTokenCfg
}

func NewAuthService(
	jwtIssuer *auth.JwtIssuer,
	rfrTokenCfg config.RefreshTokenCfg,
	transactor transactor.Transactor,
	userRps repository.UserRepository,
	rfrTknRps repository.RefreshTokenRepository,
	logger logrus.FieldLogger,
) AuthService {
	return &authService{
		jwtIssuer:   jwtIssuer,
		rfrTokenCfg: rfrTokenCfg,
		transactor:  transactor,
		userRps:     userRps,
		rfrTknRps:   rfrTknRps,
		logger:      logger,
	}
}

func (s *authService) Signup(ctx context.Context, email string, password string) (*auth.User, error) {
	existingUser, err := s.userRps.FindByEmail(ctx, email)
	if err != nil {
		s.logger.Errorf("failed to check user %s presence, read failed - %v", email, err)
		return nil, err
	}

	if existingUser != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("user with email %s already exist", email))
	}

	hash, err := auth.GeneratePasswordHash(password)
	if err != nil {
		s.logger.Errorf("password generation failed for user %s - %v", email, err)
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to generate password hash - %v", err))
	}

	u := &auth.User{
		Id:           uuid.NewString(),
		Email:        email,
		PasswordHash: hash,
	}

	if err := s.userRps.Create(ctx, u); err != nil {
		s.logger.Errorf("failed to create user %s - %v", email, err)
		return nil, err
	}
	return u, nil
}

func (s *authService) Login(ctx context.Context, email string, password string, fingerprint string, now time.Time) (jwtToken *auth.Jwt, rfrToken *auth.RefreshToken, err error) {
	err = s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		user, err := s.userRps.FindByEmail(ctx, email)
		if err != nil {
			s.logger.Errorf("failed to read user %s - %v", email, err)
			return err
		}

		if user == nil {
			s.logger.Errorf("user %s doesn't exist - unauthorized", email)
			return echo.ErrUnauthorized
		}

		if err := user.VerifyPassword(password); err != nil {
			s.logger.Errorf("user %s has provided incorrect password - unauthorized", email)
			return echo.ErrUnauthorized
		}

		jwtToken, err = s.jwtIssuer.Sign(email, now)
		if err != nil {
			s.logger.Errorf("failed to sign jwt - %v", err)
			return err
		}

		userTokens, err := s.rfrTknRps.FindTokensByUserId(ctx, user.Id)
		if err != nil {
			s.logger.Errorf("failed to read refresh tokens for user %s - %v", user.Email, err)
			return err
		}

		if len(userTokens) >= s.rfrTokenCfg.MaxCount {
			s.logger.Infof("max refresh tokens count %d is exceeded for user %s - removing all tokens before generation of new one", s.rfrTokenCfg.MaxCount, user.Email)
			if err := s.rfrTknRps.DeleteByUserId(ctx, user.Id); err != nil {
				s.logger.Errorf("failed to delete refresh tokens for user %s - %v", user.Email, err)
				return err
			}
		}

		rfrToken = s.refreshToken(user.Id, fingerprint, now)
		if err := s.rfrTknRps.Create(ctx, rfrToken); err != nil {
			s.logger.Errorf("failed to create refresh token for user %s - %v", user.Email, err)
			return err
		}

		return nil
	})

	return jwtToken, rfrToken, err
}

func (s *authService) Refresh(ctx context.Context, rfrTokenId string, fingerprint string, now time.Time) (*auth.Jwt, *auth.RefreshToken, error) {
	rfrToken, err := s.rfrTknRps.FindById(ctx, rfrTokenId)
	if err != nil {
		s.logger.Errorf("failed to read refresh token %s - %v", rfrTokenId, err)
		return nil, nil, err
	}

	if rfrToken == nil {
		s.logger.Errorf("refresh token %s doesn't exist", rfrTokenId)
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "invalid refresh token provided")
	}

	if err := s.rfrTknRps.DeleteById(ctx, rfrToken.Id); err != nil {
		s.logger.Errorf("failed to delete refresh token %s - %v", rfrTokenId, err)
		return nil, nil, err
	}

	if err := rfrToken.Verify(fingerprint, now); err != nil {
		s.logger.Errorf("refresh token %s verification failed - %v", rfrTokenId, err)
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	user, err := s.userRps.FindById(ctx, rfrToken.UserId)
	if err != nil {
		s.logger.Errorf("failed to read user with id %s - %v", rfrToken.UserId, err)
		return nil, nil, err
	}

	jwtToken, err := s.jwtIssuer.Sign(user.Email, now)
	if err != nil {
		s.logger.Errorf("failed to sign jwt - %v", err)
		return nil, nil, err
	}

	newRfrToken := s.refreshToken(user.Id, fingerprint, now)
	if err := s.rfrTknRps.Create(ctx, newRfrToken); err != nil {
		s.logger.Errorf("failed to create refresh token for user %s - %v", user.Email, err)
		return nil, nil, err
	}

	return jwtToken, newRfrToken, nil
}

func (s *authService) Logout(ctx context.Context, rfrTokenId string) error {
	if err := s.rfrTknRps.DeleteById(ctx, rfrTokenId); err != nil {
		s.logger.Errorf("failed to delete refresh token - %v", err)
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
