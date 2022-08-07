package service

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/umalmyha/customers/internal/auth"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/internal/repository/mocks"
)

const (
	jwtAlgoEd25519 = "EdDSA"
	jwtIssuerClaim = "test-issuer"
	jwtTimeToLive  = 3 * time.Minute
	jwtPrivateKey  = "MC4CAQAwBQYDK2VwBCIEIBvYJuek9MjwZuvYT+6W7S9RRgr0SmxRqejl2v6y9jjo"
)

const (
	refreshTokenMaxCount   = 2
	refreshTokenTimeToLive = 720 * time.Hour
)

type authTestData struct {
	ctx         context.Context
	now         time.Time
	password    string
	fingerprint string
	issuer      *auth.JwtIssuer
	user        *model.User
	rfrToken    *model.RefreshToken
	rfrTokenCfg *config.RefreshTokenCfg
}

type authServiceTestSuite struct {
	suite.Suite
	authSvc         AuthService
	transactorMock  *mocks.Transactor
	userRpsMock     *mocks.UserRepository
	rfrTokenRpsMock *mocks.RefreshTokenRepository
	testData        *authTestData
}

func (s *authServiceTestSuite) SetupSuite() {
	s.transactorMock = mocks.NewTransactor(s.T())
	s.transactorMock.On(
		"WithinTransaction",
		context.Background(),
		mock.AnythingOfType("func(context.Context) error"),
	).Return(func(ctx context.Context, txFunc func(ctx context.Context) error) error {
		return txFunc(ctx)
	})

	now := time.Now().UTC()
	fingerprint := "87c37298-2f3d-40a1-9438-f45d2d819206"
	password := "secret_password"

	jwtIssuer := auth.NewJwtIssuer(
		jwtIssuerClaim,
		jwt.GetSigningMethod(jwtAlgoEd25519),
		jwtTimeToLive,
		ed25519.PrivateKey(jwtPrivateKey),
	)

	user := &model.User{
		ID:           "bdf2f837-75f6-462a-b9ec-5dfb2e8f8792",
		Email:        "test@email.com",
		PasswordHash: "$2y$10$iKrALz6vQTs8KcAOElIdHeO0ZKWZkyfFnxPsJYU.Dys/2Rz177p32",
	}

	rfrToken := &model.RefreshToken{
		ID:          "1165dfc0-2dd0-4bea-ac69-4462f1cacacf",
		UserID:      user.ID,
		Fingerprint: fingerprint,
		ExpiresIn:   int(refreshTokenTimeToLive.Seconds()),
		CreatedAt:   now,
	}

	rfrTokenCfg := &config.RefreshTokenCfg{MaxCount: refreshTokenMaxCount, TimeToLive: refreshTokenTimeToLive}

	s.testData = &authTestData{
		ctx:         context.Background(),
		now:         now,
		password:    password,
		fingerprint: fingerprint,
		issuer:      jwtIssuer,
		user:        user,
		rfrToken:    rfrToken,
		rfrTokenCfg: rfrTokenCfg,
	}
}

func (s *authServiceTestSuite) SetupTest() {
	t := s.T()
	s.userRpsMock = mocks.NewUserRepository(t)
	s.rfrTokenRpsMock = mocks.NewRefreshTokenRepository(t)
	s.authSvc = NewAuthService(s.testData.issuer, s.testData.rfrTokenCfg, s.transactorMock, s.userRpsMock, s.rfrTokenRpsMock)
	s.userRpsMock.TestData()
}

func (s *authServiceTestSuite) TestSignupEmailReserved() {
	ctx := s.testData.ctx
	user := s.testData.user
	email := s.testData.user.Email
	password := s.testData.password

	s.userRpsMock.On("FindByEmail", ctx, email).Return(user, nil).Once()

	s.T().Logf("signup user %s, but email already reserved", email)
	{
		_, err := s.authSvc.Signup(ctx, email, password)
		s.Assert().Error(err, "user with email %s already exist but no error raised", email)
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestSuccessfulSignup() {
	ctx := s.testData.ctx
	email := s.testData.user.Email
	password := s.testData.password

	s.userRpsMock.On("FindByEmail", ctx, email).Return(nil, nil).Once()
	s.userRpsMock.On("Create", ctx, mock.AnythingOfType("*model.User")).Return(nil).Once()

	s.T().Logf("signup user %s and it must be signed up successfully", email)
	{
		_, err := s.authSvc.Signup(context.Background(), email, password)
		s.Assert().NoError(err, "user with email %s must be signed up successfully", email)
	}
}

func (s *authServiceTestSuite) TestLoginBadUsername() {
	ctx := s.testData.ctx
	email := s.testData.user.Email
	fingerprint := s.testData.fingerprint
	now := s.testData.now
	password := s.testData.password

	s.userRpsMock.On("FindByEmail", ctx, email).Return(nil, nil).Once()

	s.T().Logf("login user %s but email is not registered", email)
	{
		_, _, err := s.authSvc.Login(ctx, email, password, fingerprint, now)
		s.Assert().Error(err, "user with email %s is not registered, but no error raised", email)
		s.Assert().ErrorIs(err, echo.ErrUnauthorized, "it must be unauthorized error")
	}
}

func (s *authServiceTestSuite) TestLoginBadPassword() {
	ctx := s.testData.ctx
	user := s.testData.user
	email := s.testData.user.Email
	fingerprint := s.testData.fingerprint
	now := s.testData.now
	invalidPassword := "invalid_password"

	s.userRpsMock.On("FindByEmail", ctx, email).Return(user, nil).Once()

	s.T().Logf("login user %s but password is incorrect", email)
	{
		_, _, err := s.authSvc.Login(ctx, email, invalidPassword, fingerprint, now)
		s.Assert().Error(err, "wrong password is provided but no error raised")
		s.Assert().ErrorIs(err, echo.ErrUnauthorized, "it must be unauthorized error")
	}
}

func (s *authServiceTestSuite) TestLoginSuccessAndPreviousTokensRemoved() {
	ctx := s.testData.ctx
	user := s.testData.user
	email := s.testData.user.Email
	password := s.testData.password
	fingerprint := s.testData.fingerprint
	now := s.testData.now

	dbTokens := []*model.RefreshToken{
		{
			ID:          "af1adce5-51a4-4d2e-a6ba-da0e7009a1bf",
			UserID:      user.ID,
			Fingerprint: "86d36dcb-512b-402d-bec4-ae8922677cd7",
			ExpiresIn:   1000,
			CreatedAt:   now,
		},
		{
			ID:          "af1adce5-51a4-4d2e-a6ba-da0e7009a1bf",
			UserID:      user.ID,
			Fingerprint: "88a6a8ac-1104-41ae-b13c-c33deb5af5c2",
			ExpiresIn:   2000,
			CreatedAt:   now,
		},
	}

	s.userRpsMock.On("FindByEmail", ctx, email).Return(user, nil).Once()
	s.rfrTokenRpsMock.On("FindTokensByUserID", ctx, user.ID).Return(dbTokens, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByUserID", ctx, user.ID).Return(nil).Once()
	s.rfrTokenRpsMock.On("Create", ctx, mock.AnythingOfType("*model.RefreshToken")).Return(nil).Once()

	s.T().Logf("login user %s successfully, but all previous tokens will be removed", email)
	{
		jwToken, rfrToken, err := s.authSvc.Login(ctx, email, password, fingerprint, now)
		s.Assert().NoError(err, "user login is correct but error was raised")
		s.Assert().Equal(now.Add(jwtTimeToLive).Unix(), jwToken.ExpiresAt, "incorrect time to live was set for jwt")
		s.Assert().Equal(int(refreshTokenTimeToLive.Seconds()), rfrToken.ExpiresIn, "expires in is set incorrectly")
		s.rfrTokenRpsMock.AssertCalled(s.T(), "DeleteByUserID", ctx, user.ID)
	}
}

func (s *authServiceTestSuite) TestRefreshInvalidToken() {
	ctx := s.testData.ctx
	rfrToken := s.testData.rfrToken
	fingerprint := s.testData.fingerprint
	now := s.testData.now

	s.rfrTokenRpsMock.On("FindByID", ctx, rfrToken.ID).Return(nil, nil).Once()

	s.T().Log("refresh with invalid token")
	{
		_, _, err := s.authSvc.Refresh(ctx, rfrToken.ID, fingerprint, now)
		s.Assert().Error(err, "invalid refresh token id was provided but no error raised")
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestRefreshInvalidFingerprint() {
	ctx := s.testData.ctx
	rfrToken := s.testData.rfrToken
	now := s.testData.now
	invalidFingerprint := "461b07b5-3373-495d-b26b-d689a0c8a557"

	s.rfrTokenRpsMock.On("FindByID", ctx, rfrToken.ID).Return(rfrToken, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByID", ctx, rfrToken.ID).Return(nil).Once()

	s.T().Log("refresh with invalid fingerprint")
	{
		_, _, err := s.authSvc.Refresh(ctx, rfrToken.ID, invalidFingerprint, now)
		s.Assert().Error(err, "invalid refresh token fingerprint was provided but no error raised")
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestRefreshExpiredToken() {
	ctx := s.testData.ctx
	rfrToken := s.testData.rfrToken
	fingerprint := s.testData.fingerprint
	now := s.testData.now
	futureNow := now.Add(725 * time.Hour)

	s.rfrTokenRpsMock.On("FindByID", ctx, rfrToken.ID).Return(rfrToken, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByID", ctx, rfrToken.ID).Return(nil).Once()

	s.T().Log("refresh with already expired token")
	{
		_, _, err := s.authSvc.Refresh(ctx, rfrToken.ID, fingerprint, futureNow)
		s.Assert().Error(err, "refresh for expired refresh token was provided but no error raised")
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestRefreshSuccessful() {
	ctx := s.testData.ctx
	user := s.testData.user
	rfrToken := s.testData.rfrToken
	fingerprint := s.testData.fingerprint
	now := s.testData.now

	s.rfrTokenRpsMock.On("FindByID", ctx, rfrToken.ID).Return(rfrToken, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByID", ctx, rfrToken.ID).Return(nil).Once()
	s.userRpsMock.On("FindByID", ctx, rfrToken.UserID).Return(user, nil).Once()
	s.rfrTokenRpsMock.On("Create", ctx, mock.AnythingOfType("*model.RefreshToken")).Return(nil).Once()

	s.T().Log("refresh with already expired token")
	{
		jwToken, newRfrToken, err := s.authSvc.Refresh(ctx, rfrToken.ID, fingerprint, now)
		s.Assert().NoError(err, "refresh request is correctly sent but no error raised")
		s.Assert().Equal(now.Add(jwtTimeToLive).Unix(), jwToken.ExpiresAt, "incorrect time to live was set for jwt")
		s.Assert().Equal(int(refreshTokenTimeToLive.Seconds()), newRfrToken.ExpiresIn, "expires in is set incorrectly")
	}
}

func (s *authServiceTestSuite) TestLogout() {
	ctx := s.testData.ctx
	rfrToken := s.testData.rfrToken

	s.rfrTokenRpsMock.On("DeleteByID", ctx, rfrToken.ID).Return(nil).Once()

	s.T().Log("refresh with already expired token")
	{
		err := s.authSvc.Logout(ctx, rfrToken.ID)
		s.Assert().NoError(err, "logout request is correct but error was raised")
	}
}

// start auth service test suite
func TestAuthServiceTestSuite(t *testing.T) {
	suite.Run(t, new(authServiceTestSuite))
}
