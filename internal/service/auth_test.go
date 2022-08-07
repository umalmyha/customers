package service

import (
	"context"
	"crypto/ed25519"
	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/umalmyha/customers/internal/auth"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/internal/repository/mocks"
	"testing"
	"time"
)

const (
	jwtAlgoEd25519 = "EdDSA"
	jwtIssuerClaim = "test-issuer"
	jwtTimeToLive  = 3 * time.Minute
)

const (
	refreshTokenMaxCount   = 2
	refreshTokenTimeToLive = 720 * time.Hour
)

var testAuthCtx = context.Background()
var testNow = time.Now().UTC()
var testPassword = "secret_password"
var testFingerprint = "87c37298-2f3d-40a1-9438-f45d2d819206"
var testPrivateKey = ed25519.PrivateKey("MC4CAQAwBQYDK2VwBCIEIBvYJuek9MjwZuvYT+6W7S9RRgr0SmxRqejl2v6y9jjo")

var jwtIssuer = auth.NewJwtIssuer(jwtIssuerClaim, jwt.GetSigningMethod(jwtAlgoEd25519), jwtTimeToLive, testPrivateKey)
var rfrTokenCfg = &config.RefreshTokenCfg{MaxCount: refreshTokenMaxCount, TimeToLive: refreshTokenTimeToLive}

var testUser = &model.User{
	ID:           "bdf2f837-75f6-462a-b9ec-5dfb2e8f8792",
	Email:        "test@email.com",
	PasswordHash: "$2y$10$iKrALz6vQTs8KcAOElIdHeO0ZKWZkyfFnxPsJYU.Dys/2Rz177p32",
}

var testRfrToken = &model.RefreshToken{
	ID:          "1165dfc0-2dd0-4bea-ac69-4462f1cacacf",
	UserID:      testUser.ID,
	Fingerprint: testFingerprint,
	ExpiresIn:   int(refreshTokenTimeToLive.Seconds()),
	CreatedAt:   testNow,
}

type authServiceTestSuite struct {
	suite.Suite
	authSvc         AuthService
	transactorMock  *mocks.Transactor
	userRpsMock     *mocks.UserRepository
	rfrTokenRpsMock *mocks.RefreshTokenRepository
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
}

func (s *authServiceTestSuite) SetupTest() {
	t := s.T()
	s.userRpsMock = mocks.NewUserRepository(t)
	s.rfrTokenRpsMock = mocks.NewRefreshTokenRepository(t)
	s.authSvc = NewAuthService(jwtIssuer, rfrTokenCfg, s.transactorMock, s.userRpsMock, s.rfrTokenRpsMock)
}

func (s *authServiceTestSuite) TestSignupEmailReserved() {
	email := testUser.Email

	s.userRpsMock.On("FindByEmail", testAuthCtx, email).Return(testUser, nil).Once()

	s.T().Logf("signup user %s, but email already reserved", email)
	{
		_, err := s.authSvc.Signup(testAuthCtx, email, testPassword)
		s.Assert().Error(err, "user with email %s already exist but no error raised", email)
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestSuccessfulSignup() {
	email := testUser.Email

	s.userRpsMock.On("FindByEmail", testAuthCtx, email).Return(nil, nil).Once()
	s.userRpsMock.On("Create", testAuthCtx, mock.AnythingOfType("*model.User")).Return(nil).Once()

	s.T().Logf("signup user %s and it must be signed up successfully", email)
	{
		_, err := s.authSvc.Signup(context.Background(), email, testPassword)
		s.Assert().NoError(err, "user with email %s must be signed up successfully", email)
	}
}

func (s *authServiceTestSuite) TestLoginBadUsername() {
	email := testUser.Email

	s.userRpsMock.On("FindByEmail", testAuthCtx, email).Return(nil, nil).Once()

	s.T().Logf("login user %s but email is not registered", email)
	{
		_, _, err := s.authSvc.Login(testAuthCtx, email, testPassword, testFingerprint, testNow)
		s.Assert().Error(err, "user with email %s is not registered, but no error raised", email)
		s.Assert().ErrorIs(err, echo.ErrUnauthorized, "it must be unauthorized error")
	}
}

func (s *authServiceTestSuite) TestLoginBadPassword() {
	email := testUser.Email
	invalidPassword := "invalid_password"

	s.userRpsMock.On("FindByEmail", testAuthCtx, email).Return(testUser, nil).Once()

	s.T().Logf("login user %s but password is incorrect", email)
	{
		_, _, err := s.authSvc.Login(testAuthCtx, email, invalidPassword, testFingerprint, testNow)
		s.Assert().Error(err, "wrong password is provided but no error raised")
		s.Assert().ErrorIs(err, echo.ErrUnauthorized, "it must be unauthorized error")
	}
}

func (s *authServiceTestSuite) TestLoginSuccessAndPreviousTokensRemoved() {
	email := testUser.Email

	dbTokens := []*model.RefreshToken{
		{
			ID:          "af1adce5-51a4-4d2e-a6ba-da0e7009a1bf",
			UserID:      testUser.ID,
			Fingerprint: "86d36dcb-512b-402d-bec4-ae8922677cd7",
			ExpiresIn:   1000,
			CreatedAt:   testNow,
		},
		{
			ID:          "af1adce5-51a4-4d2e-a6ba-da0e7009a1bf",
			UserID:      testUser.ID,
			Fingerprint: "88a6a8ac-1104-41ae-b13c-c33deb5af5c2",
			ExpiresIn:   2000,
			CreatedAt:   testNow,
		},
	}

	s.userRpsMock.On("FindByEmail", testAuthCtx, email).Return(testUser, nil).Once()
	s.rfrTokenRpsMock.On("FindTokensByUserID", testAuthCtx, testUser.ID).Return(dbTokens, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByUserID", testAuthCtx, testUser.ID).Return(nil).Once()
	s.rfrTokenRpsMock.On("Create", testAuthCtx, mock.AnythingOfType("*model.RefreshToken")).Return(nil).Once()

	s.T().Logf("login user %s successfully, but all previous tokens will be removed", email)
	{
		jwToken, rfrToken, err := s.authSvc.Login(testAuthCtx, email, testPassword, testFingerprint, testNow)
		s.Assert().NoError(err, "user login is correct but error was raised")
		s.Assert().Equal(testNow.Add(jwtTimeToLive).Unix(), jwToken.ExpiresAt, "incorrect time to live was set for jwt")
		s.Assert().Equal(int(refreshTokenTimeToLive.Seconds()), rfrToken.ExpiresIn, "expires in is set incorrectly")
		s.rfrTokenRpsMock.AssertCalled(s.T(), "DeleteByUserID", testAuthCtx, testUser.ID)
	}
}

func (s *authServiceTestSuite) TestRefreshInvalidToken() {
	s.rfrTokenRpsMock.On("FindByID", testAuthCtx, testRfrToken.ID).Return(nil, nil).Once()

	s.T().Log("refresh with invalid token")
	{
		_, _, err := s.authSvc.Refresh(testAuthCtx, testRfrToken.ID, testFingerprint, testNow)
		s.Assert().Error(err, "invalid refresh token id was provided but no error raised")
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestRefreshInvalidFingerprint() {
	invalidFingerprint := "461b07b5-3373-495d-b26b-d689a0c8a557"

	s.rfrTokenRpsMock.On("FindByID", testAuthCtx, testRfrToken.ID).Return(testRfrToken, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByID", testAuthCtx, testRfrToken.ID).Return(nil).Once()

	s.T().Log("refresh with invalid fingerprint")
	{
		_, _, err := s.authSvc.Refresh(testAuthCtx, testRfrToken.ID, invalidFingerprint, testNow)
		s.Assert().Error(err, "invalid refresh token fingerprint was provided but no error raised")
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestRefreshExpiredToken() {
	futureNow := testNow.Add(725 * time.Hour)

	s.rfrTokenRpsMock.On("FindByID", testAuthCtx, testRfrToken.ID).Return(testRfrToken, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByID", testAuthCtx, testRfrToken.ID).Return(nil).Once()

	s.T().Log("refresh with already expired token")
	{
		_, _, err := s.authSvc.Refresh(testAuthCtx, testRfrToken.ID, testFingerprint, futureNow)
		s.Assert().Error(err, "refresh for expired refresh token was provided but no error raised")
		s.Assert().IsType(&echo.HTTPError{}, err, "error must be echo error")
	}
}

func (s *authServiceTestSuite) TestRefreshSuccessful() {
	s.rfrTokenRpsMock.On("FindByID", testAuthCtx, testRfrToken.ID).Return(testRfrToken, nil).Once()
	s.rfrTokenRpsMock.On("DeleteByID", testAuthCtx, testRfrToken.ID).Return(nil).Once()
	s.userRpsMock.On("FindByID", testAuthCtx, testRfrToken.UserID).Return(testUser, nil).Once()
	s.rfrTokenRpsMock.On("Create", testAuthCtx, mock.AnythingOfType("*model.RefreshToken")).Return(nil).Once()

	s.T().Log("refresh with already expired token")
	{
		jwToken, rfrToken, err := s.authSvc.Refresh(testAuthCtx, testRfrToken.ID, testFingerprint, testNow)
		s.Assert().NoError(err, "refresh request is correctly sent but no error raised")
		s.Assert().Equal(testNow.Add(jwtTimeToLive).Unix(), jwToken.ExpiresAt, "incorrect time to live was set for jwt")
		s.Assert().Equal(int(refreshTokenTimeToLive.Seconds()), rfrToken.ExpiresIn, "expires in is set incorrectly")
	}
}

func (s *authServiceTestSuite) TestLogout() {
	s.rfrTokenRpsMock.On("DeleteByID", testAuthCtx, testRfrToken.ID).Return(nil).Once()

	s.T().Log("refresh with already expired token")
	{
		err := s.authSvc.Logout(testAuthCtx, testRfrToken.ID)
		s.Assert().NoError(err, "logout request is correct but error was raised")
	}
}

// start auth service test suite
func TestAuthServiceTestSuite(t *testing.T) {
	suite.Run(t, new(authServiceTestSuite))
}
