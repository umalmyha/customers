package handlers

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v9"
	"github.com/golang-jwt/jwt/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/suite"
	"github.com/umalmyha/customers/internal/auth"
	"github.com/umalmyha/customers/internal/cache"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/internal/validation"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"github.com/umalmyha/customers/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const grpcConnBufSize = 1024 * 1024

const (
	connectionTimeout = 3 * time.Second
	testNetwork       = "customers-handlers-test-net"
)

const (
	pgContainerName = "pg-handlers-test-customers"
	pgPort          = "5432"
	pgTestUser      = "handlers-test"
	pgTestPassword  = "handlers-test"
	pgTestDB        = "handlers-customers"
)

const (
	redisContainerName = "redis-handlers-test-customers"
	redisTestPassword  = "handlers-test"
	redisPort          = "6379"
	redisTestDB        = 0
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

const (
	testEmail       = "testemail@email.com"
	testFingerprint = "96b46194-5ba5-4aa5-a342-c1075354427e"
	testPassword    = "secret_password"
)

type handlersDockerResources struct {
	postgres *dockertest.Resource
	redis    *dockertest.Resource
	network  *docker.Network
}

type handlersTestSuite struct {
	suite.Suite
	app         *echo.Echo
	authSvc     service.AuthService
	customerSvc service.CustomerService
	dockerPool  *dockertest.Pool
	resources   handlersDockerResources
	pgPool      *pgxpool.Pool
	redisClient *redis.Client
	bufListener *bufconn.Listener
	bufDialer   func(context.Context, string) (net.Conn, error)
}

//nolint:funlen // function contains a lot of boilerplate actions
func (s *handlersTestSuite) SetupSuite() {
	t := s.T()
	assert := s.Require()

	// build docker pool
	t.Log("build docker pool")
	dockerPool, err := dockertest.NewPool("")
	assert.NoError(err, "failed to create pool")

	t.Log("sending ping to docker...")
	err = dockerPool.Client.Ping()
	assert.NoError(err, "failed to connect to docker")

	s.dockerPool = dockerPool // assign pool

	// create network for containers
	t.Log("creating network...")
	network, err := dockerPool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: testNetwork})
	assert.NoError(err, "failed to create network")

	s.resources.network = network // assign network

	// start postgres
	t.Log("starting postgres container...")
	postgres, err := dockerPool.RunWithOptions(&dockertest.RunOptions{
		Name:       pgContainerName,
		Repository: "postgres",
		Tag:        "latest",
		NetworkID:  network.ID,
		Env: []string{
			fmt.Sprintf("POSTGRES_USER=%s", pgTestUser),
			fmt.Sprintf("POSTGRES_PASSWORD=%s", pgTestPassword),
			fmt.Sprintf("POSTGRES_DB=%s", pgTestDB),
		},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"5432/tcp": {{HostIP: "localhost", HostPort: fmt.Sprintf("%s/tcp", pgPort)}},
		},
	})
	assert.NoError(err, "failed to start postgresql")

	// run migrations
	t.Log("run flyway migrations...")
	flywayCmd := []string{
		fmt.Sprintf("-url=jdbc:postgresql://%s:%s/%s", pgContainerName, pgPort, pgTestDB),
		fmt.Sprintf("-user=%s", pgTestUser),
		fmt.Sprintf("-password=%s", pgTestPassword),
		"-connectRetries=10",
		"migrate",
	}

	migrationsPath, err := filepath.Abs("../../migrations")
	assert.NoError(err, "failed to build path to flyway migrations")

	flywayMounts := []string{fmt.Sprintf("%s:/flyway/sql", migrationsPath)}

	flyway, err := dockerPool.RunWithOptions(&dockertest.RunOptions{
		Repository: "flyway/flyway",
		Tag:        "latest",
		NetworkID:  network.ID,
		Cmd:        flywayCmd,
		Mounts:     flywayMounts,
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
	})
	assert.NoError(err, "failed to start flyway migrations")

	s.resources.postgres = postgres // assign postgres

	// waiting for flyway container to be destroyed
	err = dockerPool.Retry(func() error {
		if _, ok := dockerPool.ContainerByName(flyway.Container.Name); ok {
			return errors.New("flyway migrations are still in progress")
		}
		return nil
	})
	assert.NoError(err, "failed to await flyway migrations")

	// connect to postgres
	t.Log("connecting to postgres...")
	pgURI := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", pgTestUser, pgTestPassword, pgPort, pgTestDB)
	err = dockerPool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
		defer cancel()

		var e error
		s.pgPool, e = pgxpool.Connect(ctx, pgURI)
		if e != nil {
			return e
		}
		return s.pgPool.Ping(ctx)
	})
	assert.NoError(err, "failed to establish connection to postgresql")

	t.Log("starting redis...")
	redisCache, err := dockerPool.RunWithOptions(&dockertest.RunOptions{
		Name:       redisContainerName,
		Repository: "redis",
		Tag:        "latest",
		NetworkID:  network.ID,
		Cmd:        []string{fmt.Sprintf("--requirepass %s", redisTestPassword)},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"6379/tcp": {{HostIP: "localhost", HostPort: fmt.Sprintf("%s/tcp", redisPort)}},
		},
	})
	assert.NoError(err, "failed to start redis")

	s.resources.redis = redisCache // assign redis

	// connect to redis
	t.Log("connecting to redis...")
	err = dockerPool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
		defer cancel()

		s.redisClient = redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("localhost:%s", redisPort),
			Password: redisTestPassword,
			DB:       redisTestDB,
		})

		return s.redisClient.Ping(ctx).Err()
	})
	assert.NoError(err, "failed to establish connection to redis")

	// create validator for echo
	enLocale := en.New()
	unvTranslator := ut.New(enLocale, enLocale)
	trans, ok := unvTranslator.GetTranslator("en")
	if !ok {
		assert.Fail("failed to build echo validator because of missing en translations")
	}

	// create echo app instance
	s.app = echo.New()
	s.app.Validator = validation.Echo(validator.New(), trans)

	// create service dependencies
	jwtIssuer := auth.NewJwtIssuer(jwtIssuerClaim, jwt.GetSigningMethod(jwtAlgoEd25519), jwtTimeToLive, ed25519.PrivateKey(jwtPrivateKey))
	rfrTokenCfg := &config.RefreshTokenCfg{MaxCount: refreshTokenMaxCount, TimeToLive: refreshTokenTimeToLive}

	txExecutor := transactor.NewPgxWithinTransactionExecutor(s.pgPool)
	userRps := repository.NewPostgresUserRepository(txExecutor)
	rfrTokenRps := repository.NewPostgresRefreshTokenRepository(txExecutor)
	customerRps := repository.NewPostgresCustomerRepository(s.pgPool)
	customerCache := cache.NewRedisCustomerCache(s.redisClient)

	s.authSvc = service.NewAuthService(jwtIssuer, rfrTokenCfg, transactor.NewPgxTransactor(s.pgPool), userRps, rfrTokenRps)
	s.customerSvc = service.NewCustomerService(customerRps, customerCache)

	// start gRPC server
	s.bufListener = bufconn.Listen(grpcConnBufSize)

	authGrpcHandler := NewAuthGrpcHandler(s.authSvc)
	customerGrpcHandler := NewCustomerGrpcHandler(s.customerSvc)

	server := grpc.NewServer()
	proto.RegisterAuthServiceServer(server, authGrpcHandler)
	proto.RegisterCustomerServiceServer(server, customerGrpcHandler)

	go func() {
		if err := server.Serve(s.bufListener); err != nil {
			s.Require().Failf("suite setup failed", "failed to start gRPC server - %v", err)
		}
	}()

	s.bufDialer = func(context.Context, string) (net.Conn, error) {
		return s.bufListener.Dial()
	}
}

func (s *handlersTestSuite) TearDownSuite() {
	t := s.T()

	if s.pgPool != nil {
		t.Log("closing connection to postgres")
		s.pgPool.Close()
	}

	if s.redisClient != nil {
		t.Log("closing connection to redis")
		if err := s.redisClient.Close(); err != nil {
			t.Logf("failed to gracefully close connection to redis - %v", err)
		}
	}

	resources := s.resources

	if resources.postgres != nil {
		if err := s.dockerPool.Purge(resources.postgres); err != nil {
			t.Logf("failed to purge postgres container - %v", err)
		}
	}

	if resources.redis != nil {
		if err := s.dockerPool.Purge(resources.redis); err != nil {
			t.Logf("failed to purge redis container - %v", err)
		}
	}

	if resources.network != nil {
		if err := s.dockerPool.Client.RemoveNetwork(resources.network.ID); err != nil {
			t.Logf("failed to delete network - %v", err)
		}
	}
}

//nolint:funlen // function contains a lot of inlined tests
func (s *handlersTestSuite) TestAuthHTTPHandler() {
	t := s.T()
	require := s.Require()

	var sess session
	authHTTPHandler := NewAuthHTTPHandler(s.authSvc)

	t.Log("signup with wrong payload")
	{
		wrongPayloadJSON := `{"email":"testemail.ema`
		c, _ := s.echoPostContext("/api/auth/signup", wrongPayloadJSON)
		err := authHTTPHandler.Signup(c)
		require.Error(err, "wrong payload has been provided but no error raised")
		require.IsType(&echo.HTTPError{}, err, "error must be echo error")
	}

	t.Log("signup with invalid data sent in payload")
	{
		invalidJSON := fmt.Sprintf(`{"email":"testemail.email.com","password":%q}`, testPassword)
		c, _ := s.echoPostContext("/api/auth/signup", invalidJSON)
		err := authHTTPHandler.Signup(c)
		require.Error(err, "invalid data in payload has been provided but no error raised")
		require.IsType(&validation.PayloadError{}, err, "error must be payload error")
	}

	t.Log("successful signup")
	{
		signupJSON := fmt.Sprintf(`{"email":%q,"password":%q}"`, testEmail, testPassword)
		c, rec := s.echoPostContext("/api/auth/signup", signupJSON)
		err := authHTTPHandler.Signup(c)
		require.NoError(err, "no error must be raised")
		require.Equal(http.StatusOK, rec.Code, "response status code must be OK")
	}

	t.Log("login with wrong payload")
	{
		wrongPayloadJSON := `{"email":"testemail.email.c`
		c, _ := s.echoPostContext("/api/auth/login", wrongPayloadJSON)
		err := authHTTPHandler.Login(c)
		require.Error(err, "wrong payload has been provided but no error raised")
		require.IsType(&echo.HTTPError{}, err, "error must be echo error")
	}

	t.Log("login with invalid data in payload")
	{
		invalidJSON := `{"email":"testemail.email.com","password":"","fingerprint":""}`
		c, _ := s.echoPostContext("/api/auth/login", invalidJSON)
		err := authHTTPHandler.Login(c)
		require.Error(err, "wrong data in payload has been provided but no error raised")
		require.IsType(&validation.PayloadError{}, err, "error must be payload error")
	}

	t.Log("login with wrong password")
	{
		wrongCredsJSON := fmt.Sprintf(`{"email":%q,"password":"wrong","fingerprint":%q}`, testEmail, testFingerprint)
		c, _ := s.echoPostContext("/api/auth/login", wrongCredsJSON)
		err := authHTTPHandler.Login(c)
		require.Error(err, "wrong credentials have been provided but no error raised")
		require.ErrorIs(err, echo.ErrUnauthorized, "code must be unauthorized")
	}

	t.Log("successful login")
	{
		loginJSON := fmt.Sprintf(`{"email":%q,"password":%q,"fingerprint":%q}`, testEmail, testPassword, testFingerprint)
		c, rec := s.echoPostContext("/api/auth/login", loginJSON)
		err := authHTTPHandler.Login(c)
		require.NoError(err, "no error must be raised")
		require.Equal(http.StatusOK, rec.Code, "response status code must be OK")

		if err := json.NewDecoder(rec.Body).Decode(&sess); err != nil {
			require.NoError(err, "failed to parse session from response")
		}
	}

	t.Log("refresh with wrong payload")
	{
		wrongPayloadJSON := `{"fingerprint":"1111`
		c, _ := s.echoPostContext("/api/auth/refresh", wrongPayloadJSON)
		err := authHTTPHandler.Refresh(c)
		require.Error(err, "wrong payload has been provided but no error raised")
		require.IsType(&echo.HTTPError{}, err, "error must be echo error")
	}

	t.Log("refresh with invalid data in payload")
	{
		invalidJSON := `{"fingerprint":"11111","refreshToken":""}`
		c, _ := s.echoPostContext("/api/auth/refresh", invalidJSON)
		err := authHTTPHandler.Refresh(c)
		require.Error(err, "wrong data in payload has been provided but no error raised")
		require.IsType(&validation.PayloadError{}, err, "error must be payload error")
	}

	t.Log("successful refresh")
	{
		refreshJSON := fmt.Sprintf(`{"fingerprint":%q,"refreshToken":%q}`, testFingerprint, sess.RefreshToken)
		c, rec := s.echoPostContext("/api/auth/refresh", refreshJSON)
		err := authHTTPHandler.Refresh(c)
		require.NoError(err, "refresh request is correct but error raised")
		require.Equal(http.StatusOK, rec.Code, "response status code must be OK")
	}

	t.Log("logout with wrong payload")
	{
		wrongPayloadJSON := `{"refreshToken":"`
		c, _ := s.echoPostContext("/api/auth/logout", wrongPayloadJSON)
		err := authHTTPHandler.Logout(c)
		require.Error(err, "wrong payload has been provided but no error raised")
		require.IsType(&echo.HTTPError{}, err, "error must be echo error")
	}

	t.Log("logout with invalid data in payload")
	{
		invalidJSON := `{"refreshToken":"1111"}`
		c, _ := s.echoPostContext("/api/auth/logout", invalidJSON)
		err := authHTTPHandler.Logout(c)
		require.Error(err, "wrong data in payload has been provided but no error raised")
		require.IsType(&validation.PayloadError{}, err, "error must be payload error")
	}

	t.Log("successful logout")
	{
		logoutJSON := fmt.Sprintf(`{"refreshToken":%q}`, sess.RefreshToken)
		c, rec := s.echoPostContext("/api/auth/logout", logoutJSON)
		err := authHTTPHandler.Logout(c)
		require.NoError(err, "refresh request is correct but error raised")
		require.Equal(http.StatusOK, rec.Code, "response status code must be OK")
	}
}

//nolint:funlen // function contains a lot of inlined tests
func (s *handlersTestSuite) TestCustomerHTTPHandler() {
	t := s.T()
	require := s.Require()

	customerRps := repository.NewPostgresCustomerRepository(s.pgPool)
	redisCacheRps := cache.NewRedisCustomerCache(s.redisClient)

	customerSvc := service.NewCustomerService(customerRps, redisCacheRps)
	customerHTTPHandler := NewCustomerHTTPHandler(customerSvc)

	testID := "7b45dbaa-ddf8-4ded-b858-78be123b3e6f"

	t.Log("post customer with wrong payload")
	{
		wrongPayloadJSON := `{
   			"firstName":"John",
   			"lastName":"Smith",
   			"middleName":null,
   			"email":"john.smith@testapi.com",
   			"importance,
   			"inactive":false
		}`

		c, _ := s.echoPostContext("/api/v1/customers", wrongPayloadJSON)
		err := customerHTTPHandler.Post(c)
		require.Error(err, "wrong payload has been provided but no error raised")
		require.IsType(&echo.HTTPError{}, err, "error must be echo error")
	}

	t.Log("post customer with invalid data in payload")
	{
		invalidJSON := `{
   			"firstName":"John",
   			"lastName":"Smith",
   			"middleName":null,
   			"email":"john.smith-api.com",
   			"importance": 2,
   			"inactive":false
		}`

		c, _ := s.echoPostContext("/api/v1/customers", invalidJSON)
		err := customerHTTPHandler.Post(c)
		require.Error(err, "wrong data in payload has been provided but no error raised")
		require.IsType(&validation.PayloadError{}, err, "error must be payload error")
	}

	t.Log("post customer successfully")
	{
		postCustomer := `{
   			"firstName":"John",
   			"lastName":"Smith",
   			"middleName":null,
   			"email":"john.smith@testapi.com",
   			"importance": 2,
   			"inactive":false
		}`

		c, rec := s.echoPostContext("/api/v1/customers", postCustomer)
		err := customerHTTPHandler.Post(c)
		require.NoError(err, "no error must be raised")
		require.Equal(http.StatusCreated, rec.Code, "response code must be Created")
	}

	t.Log("put customer with wrong payload")
	{
		wrongPayloadJSON := `{
			"firstName":"John",
			"lastName":"Smith",
			"middle,
			"email":"john.smith@testapi.com",
			"importance,
			"inactive":false
		}`

		c, _ := s.echoPutContext(fmt.Sprintf("/api/v1/customers/%s", testID), testID, wrongPayloadJSON)
		err := customerHTTPHandler.Put(c)
		require.Error(err, "wrong payload has been provided but no error raised")
		require.IsType(&echo.HTTPError{}, err, "error must be echo error")
	}

	t.Log("put customer with invalid data in payload")
	{
		invalidJSON := `{
			"firstName":"John",
			"lastName":"Smith",
			"middleName":null,
			"email":"john.smithtestapi.com",
			"importance": 2,
			"inactive":false
		}`

		c, _ := s.echoPutContext(fmt.Sprintf("/api/v1/customers/%s", testID), testID, invalidJSON)
		err := customerHTTPHandler.Put(c)
		require.Error(err, "wrong data in payload has been provided but no error raised")
		require.IsType(&validation.PayloadError{}, err, "error must be payload error")
	}

	t.Log("put customer successfully")
	{
		putCustomer := `{
			"firstName":"John",
			"lastName":"Smith",
			"middleName":null,
			"email":"john.smith@testapi.com",
			"importance": 2,
			"inactive":false
		}`

		c, rec := s.echoPutContext(fmt.Sprintf("/api/v1/customers/%s", testID), testID, putCustomer)
		err := customerHTTPHandler.Put(c)
		require.NoError(err, "no error must be raised")
		require.Equal(http.StatusOK, rec.Code, "response code must be OK")
	}

	t.Log("get customer by id with wrong uuid format")
	{
		c, _ := s.echoGetContext(fmt.Sprintf("/api/v1/customers/%s", "1111"))
		c.SetParamNames("id")
		c.SetParamValues("1111")
		err := customerHTTPHandler.Get(c)
		require.Error(err, "wrong data in payload has been provided but no error raised")
		require.IsType(&validation.PayloadError{}, err, "error must be payload error")
	}

	t.Log("get customer by id successfully")
	{
		c, rec := s.echoGetContext(fmt.Sprintf("/api/v1/customers/%s", testID))
		c.SetParamNames("id")
		c.SetParamValues(testID)
		err := customerHTTPHandler.Get(c)
		require.NoError(err, "no error must be raised")
		require.Equal(http.StatusOK, rec.Code, "response status must be OK")
	}

	t.Log("get all customers successfully")
	{
		c, rec := s.echoGetContext("/api/v1/customers")
		err := customerHTTPHandler.GetAll(c)
		require.NoError(err, "no error must be raised")
		require.Equal(http.StatusOK, rec.Code, "response status must be OK")
	}

	t.Log("delete customer by id")
	{
		c, rec := s.echoDeleteContext("/api/v1/customers", testID)
		err := customerHTTPHandler.DeleteByID(c)
		require.NoError(err, "no error must be raised")
		require.Equal(http.StatusNoContent, rec.Code, "response status must be OK")
	}
}

func (s *handlersTestSuite) TestAuthGrpcHandler() {
	t := s.T()
	require := s.Require()
	grpcTestEmail := "sometest@someemail.com"

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(s.bufDialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(err, "failed to create gRPC connection")
	defer conn.Close()

	client := proto.NewAuthServiceClient(conn)

	var rfrToken string

	t.Log("signup new user")
	_, err = client.Signup(ctx, &proto.SignupRequest{
		Email:    grpcTestEmail,
		Password: testPassword,
	})
	require.NoError(err, "no error must be raised")

	t.Log("login with recently created user")
	sess, err := client.Login(ctx, &proto.LoginRequest{
		Email:       grpcTestEmail,
		Password:    testPassword,
		Fingerprint: testFingerprint,
	})
	require.NoError(err, "no error must be raised")
	rfrToken = sess.RefreshToken

	t.Log("refresh session")
	_, err = client.Refresh(ctx, &proto.RefreshRequest{
		Fingerprint:  testFingerprint,
		RefreshToken: rfrToken,
	})
	require.NoError(err, "no error must be raised")

	t.Log("logout")
	_, err = client.Logout(ctx, &proto.LogoutRequest{RefreshToken: rfrToken})
	require.NoError(err, "no error must be raised")
}

func (s *handlersTestSuite) TestCustomerGrpcHandler() {
	t := s.T()
	require := s.Require()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(s.bufDialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(err, "failed to create gRPC connection")
	defer conn.Close()

	testID := "e7be204e-b693-4b99-b067-2eae1610b3ee"

	client := proto.NewCustomerServiceClient(conn)

	t.Log("create customer")
	_, err = client.Create(ctx, &proto.NewCustomerRequest{
		FirstName:  "John",
		LastName:   "Smith",
		MiddleName: nil,
		Email:      "john.smith@testapi.com",
		Importance: proto.CustomerImportance_HIGH,
		Inactive:   false,
	})
	require.NoError(err, "no error must be raised")

	t.Log("put new customer")
	_, err = client.Upsert(ctx, &proto.UpdateCustomerRequest{
		Id:         testID,
		FirstName:  "John",
		LastName:   "Smith",
		MiddleName: nil,
		Email:      "john.smith@testapi.com",
		Importance: proto.CustomerImportance_HIGH,
		Inactive:   false,
	})
	require.NoError(err, "no error must be raised")

	t.Log("get recently created customer")
	c, err := client.GetByID(ctx, &proto.GetCustomerByIdRequest{Id: testID})
	require.NoError(err, "no error must be raised")
	require.Equal(testID, c.Id, "incorrect customer was returned")

	t.Log("delete customer by id")
	_, err = client.DeleteByID(ctx, &proto.DeleteCustomerByIdRequest{Id: testID})
	require.NoError(err, "no error must be raised")

	t.Log("get all customers")
	list, err := client.GetAll(ctx, new(emptypb.Empty))
	require.NoError(err, "no error must be raised")
	require.NotEqual(0, len(list.Customers), "incorrect number of customers returned")
}

func (s *handlersTestSuite) echoPostContext(target, payload string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return s.app.NewContext(req, rec), rec
}

func (s *handlersTestSuite) echoGetContext(target string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, target, strings.NewReader(""))
	rec := httptest.NewRecorder()
	return s.app.NewContext(req, rec), rec
}

func (s *handlersTestSuite) echoDeleteContext(target, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodDelete, target, strings.NewReader(""))
	rec := httptest.NewRecorder()
	c := s.app.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func (s *handlersTestSuite) echoPutContext(target, id, payload string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := s.app.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

// start handlers test suite
func TestHandlersTestSuite(t *testing.T) {
	suite.Run(t, new(handlersTestSuite))
}
