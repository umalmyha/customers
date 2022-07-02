package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/umalmyha/customers/internal/domain/auth"
	"github.com/umalmyha/customers/internal/repository"
)

type AuthService interface {
	Signup(context.Context, auth.Signup) (auth.User, error)
	Login(context.Context, auth.Login) (auth.Jwt, auth.RefreshToken, error)
	Logout(context.Context, string) error
	Refresh(context.Context, auth.Refresh) (auth.Jwt, auth.RefreshToken, error)
}

type authService struct {
	userRepo       repository.UserRepository
	rfrTknRepo     repository.RefreshTokenRepository
	jwtIssuer      *auth.JwtIssuer
	rfrTokenIssuer *auth.RefreshTokenIssuer
}

func NewAuthService(
	jwtIssuer *auth.JwtIssuer,
	rfrTokenIssuer *auth.RefreshTokenIssuer,
	userRepo repository.UserRepository,
	rfrTknRepo repository.RefreshTokenRepository,
) AuthService {
	return &authService{jwtIssuer: jwtIssuer, rfrTokenIssuer: rfrTokenIssuer, userRepo: userRepo, rfrTknRepo: rfrTknRepo}
}

func (s *authService) Signup(ctx context.Context, signup auth.Signup) (auth.User, error) {
	// TODO: Additional validations
	if err := signup.ValidatePasswords(); err != nil {
		return auth.User{}, err
	}

	hash, err := auth.GeneratePasswordHash(signup.Password)
	if err != nil {
		return auth.User{}, err
	}

	u := auth.User{
		Id:           uuid.NewString(),
		Email:        signup.Email,
		PasswordHash: hash,
	}

	if err := s.userRepo.Create(ctx, u); err != nil {
		return auth.User{}, err
	}
	return u, nil
}

func (s *authService) Login(ctx context.Context, login auth.Login) (auth.Jwt, auth.RefreshToken, error) {
	user, err := s.userRepo.FindByEmail(ctx, login.Email)
	if err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}

	if user.Id == "" {
		return auth.Jwt{}, auth.RefreshToken{}, fmt.Errorf("unknown user with email %s", login.Email) // TODO: Raise Unauthorized
	}

	if err := user.VerifyPassword(login.Password); err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, errors.New("password is incorrect") // TODO: Raise Unauthorized
	}

	jwtToken, err := s.jwtIssuer.Sign(login.Email, login.At)
	if err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}

	rfrToken := s.rfrTokenIssuer.Sign(user.Id, login.Fingerprint, login.At)

	userTkns, err := s.rfrTknRepo.FindTokensByUserId(ctx, user.Id)
	if len(userTkns) > s.rfrTokenIssuer.TokensMaxCount() {
		if err := s.rfrTknRepo.DeleteByUserId(ctx, user.Id); err != nil {
			return auth.Jwt{}, auth.RefreshToken{}, err
		}
	}

	if err := s.rfrTknRepo.Create(ctx, rfrToken); err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}
	return jwtToken, rfrToken, nil
}

func (s *authService) Refresh(ctx context.Context, refresh auth.Refresh) (auth.Jwt, auth.RefreshToken, error) {
	rfrToken, err := s.rfrTknRepo.FindById(ctx, refresh.Token)
	if err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}

	if rfrToken.Id == "" {
		return auth.Jwt{}, auth.RefreshToken{}, errors.New("non-existent refresh token provided")
	}

	if err := s.rfrTknRepo.DeleteById(ctx, rfrToken.Id); err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}

	if err := rfrToken.Verify(refresh.Fingerprint, refresh.At); err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}

	user, err := s.userRepo.FindById(ctx, rfrToken.UserId)
	if err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}

	jwtToken, err := s.jwtIssuer.Sign(user.Email, refresh.At)
	if err != nil {
		return auth.Jwt{}, auth.RefreshToken{}, err
	}

	newRfrToken := s.rfrTokenIssuer.Sign(user.Id, refresh.Fingerprint, refresh.At)

	return jwtToken, newRfrToken, nil
}

func (s *authService) Logout(ctx context.Context, tokenId string) error {
	if err := s.rfrTknRepo.DeleteById(ctx, tokenId); err != nil {
		return err
	}
	return nil
}
