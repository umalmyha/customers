package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/suite"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/ory/dockertest/v3"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	connectionTimeout = 3 * time.Second
	testCtxTimeout    = 10 * time.Second
	testNetwork       = "customers-rps-test-net"
)

const (
	pgContainerName = "pg-rps-test-customers"
	pgPort          = "5432"
	pgTestUser      = "rps-test"
	pgTestPassword  = "rps-test"
	pgTestDB        = "rps-customers"
)

const (
	mongoContainerName = "mongo-rps-test-customers"
	mongoPort          = "27017"
	mongoTestUser      = "rps-test"
	mongoTestPassword  = "rps-test"
)

type repositoryDockerResources struct {
	postgres *dockertest.Resource
	mongodb  *dockertest.Resource
	network  *docker.Network
}

type repositoryTestSuite struct {
	suite.Suite
	dockerPool  *dockertest.Pool
	resources   repositoryDockerResources
	pgPool      *pgxpool.Pool
	mongoClient *mongo.Client
}

func (s *repositoryTestSuite) SetupSuite() {
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
	pgUri := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", pgTestUser, pgTestPassword, pgPort, pgTestDB)
	err = dockerPool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
		defer cancel()

		var err error
		s.pgPool, err = pgxpool.Connect(ctx, pgUri)
		if err != nil {
			return err
		}
		return s.pgPool.Ping(ctx)
	})
	assert.NoError(err, "failed to establish connection to postgresql")

	// start mongo
	t.Log("starting mongodb...")
	mongodb, err := dockerPool.RunWithOptions(&dockertest.RunOptions{
		Name:       mongoContainerName,
		Repository: "mongo",
		Tag:        "latest",
		NetworkID:  network.ID,
		Env: []string{
			fmt.Sprintf("MONGO_INITDB_ROOT_USERNAME=%s", mongoTestUser),
			fmt.Sprintf("MONGO_INITDB_ROOT_PASSWORD=%s", mongoTestPassword),
		},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"27017/tcp": {{HostIP: "localhost", HostPort: fmt.Sprintf("%s/tcp", mongoPort)}},
		},
	})
	assert.NoError(err, "failed to start mongodb")

	s.resources.mongodb = mongodb // assign mongodb

	// connect to mongo
	t.Log("connecting to mongodb...")
	mongoUri := fmt.Sprintf("mongodb://%s:%s@localhost:%s", mongoTestUser, mongoTestPassword, mongoPort)
	err = dockerPool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
		defer cancel()

		var err error
		s.mongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoUri))
		if err != nil {
			return err
		}
		return s.mongoClient.Ping(ctx, readpref.Primary())
	})
	assert.NoError(err, "failed to establish connection to mongodb")
}

func (s *repositoryTestSuite) TearDownSuite() {
	t := s.T()

	if s.pgPool != nil {
		t.Log("closing connection to postgres")
		s.pgPool.Close()
	}

	if s.mongoClient != nil {
		t.Log("closing connection to mongodb")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := s.mongoClient.Disconnect(ctx); err != nil {
			t.Logf("failed to gracefully close connection to mongodb - %v", err)
		}
		cancel()
	}

	resources := s.resources

	if resources.postgres != nil {
		if err := s.dockerPool.Purge(resources.postgres); err != nil {
			t.Logf("failed to purge postgres container - %v", err)
		}
	}

	if resources.mongodb != nil {
		if err := s.dockerPool.Purge(resources.mongodb); err != nil {
			t.Logf("failed to purge mongodb container - %v", err)
		}
	}

	if resources.network != nil {
		if err := s.dockerPool.Client.RemoveNetwork(resources.network.ID); err != nil {
			t.Logf("failed to delete network - %v", err)
		}
	}
}

func (s *repositoryTestSuite) TestUserRps() {
	t := s.T()
	require := s.Require()

	ctx, cancel := context.WithTimeout(context.Background(), testCtxTimeout)
	defer cancel()

	userRps := NewPostgresUserRepository(transactor.NewPgxWithinTransactionExecutor(s.pgPool))

	u := &model.User{
		ID:           "f9771714-df35-4186-b1f1-57fba3e5d3f2",
		Email:        "customer1@somemail.com",
		PasswordHash: "f929cb58673be0a35fcb22ad7f147bd1",
	}

	t.Log("create user")
	{
		err := userRps.Create(ctx, u)
		require.NoError(err, "failed to create user")
	}

	t.Log("find user by id")
	{
		dbUser, err := userRps.FindByID(ctx, u.ID)
		require.NoError(err, "failed to read user by id")
		require.NotNil(dbUser, "user was created recently but not found by id")
	}

	t.Log("find user by email")
	{
		dbUser, err := userRps.FindByEmail(ctx, u.Email)
		require.NoError(err, "failed to read user by email")
		require.NotNil(dbUser, "user was created recently but not found by email")
	}

	t.Log("create user duplicate")
	{
		err := userRps.Create(ctx, u)
		require.Error(err, "aimed to create user duplicate but no error raised")
	}
}

func (s *repositoryTestSuite) TestRefreshTokenRps() {
	t := s.T()
	require := s.Require()

	ctx, cancel := context.WithTimeout(context.Background(), testCtxTimeout)
	defer cancel()

	expiresIn := 3000
	fingerprint := "b86de171-7481-4b57-a012-765e6e34e2c2"
	createdAt := time.Now().UTC()

	userRps := NewPostgresUserRepository(transactor.NewPgxWithinTransactionExecutor(s.pgPool))
	rfrTokenRps := NewPostgresRefreshTokenRepository(transactor.NewPgxWithinTransactionExecutor(s.pgPool))

	userJohn := &model.User{
		ID:           "afa94457-c29a-4569-a4aa-0ae3b7e5a255",
		Email:        "john@somemail.com",
		PasswordHash: "7c9fb260749f6d1cf54530450ac97f72",
	}

	userHenry := &model.User{
		ID:           "0583d7f3-5ae1-416a-92fa-120851905551",
		Email:        "henry@somemail.com",
		PasswordHash: "966ac2a7543413f3368a2fc3ca889f98",
	}

	// john has 2 tokens and henry has 1 token
	refreshTokens := []*model.RefreshToken{
		{
			ID:          "19264f8d-8862-47e0-9892-44930e2de59f",
			UserID:      userJohn.ID,
			Fingerprint: fingerprint,
			ExpiresIn:   expiresIn,
			CreatedAt:   createdAt,
		},
		{
			ID:          "55ed2faa-de40-4344-a512-0ffbc43d4184",
			UserID:      userJohn.ID,
			Fingerprint: fingerprint,
			ExpiresIn:   expiresIn,
			CreatedAt:   createdAt,
		},
		{
			ID:          "112a54c0-e744-4712-8acf-59e6b1a386e5",
			UserID:      userHenry.ID,
			Fingerprint: fingerprint,
			ExpiresIn:   expiresIn,
			CreatedAt:   createdAt,
		},
	}

	henryToken := refreshTokens[2]

	t.Log("reference users must be added")
	{
		err := userRps.Create(ctx, userJohn)
		require.NoError(err, "failed to create user %s", userJohn.Email)

		err = userRps.Create(ctx, userHenry)
		require.NoError(err, "failed to create user %s", userHenry.Email)
	}

	t.Logf("create %d tokens", len(refreshTokens))
	{
		for _, tkn := range refreshTokens {
			err := rfrTokenRps.Create(ctx, tkn)
			require.NoError(err, "failed to create token %s", tkn.ID)
		}
	}

	t.Logf("find tokens for user %s", userJohn.Email)
	{
		johnDBTokens, err := rfrTokenRps.FindTokensByUserID(ctx, userJohn.ID)
		require.NoError(err, "failed to read tokens")
		expected := 2
		actual := len(johnDBTokens)
		require.Equal(expected, actual, "%d tokens where created for user %s, got %d", expected, userJohn.Email, actual)
	}

	t.Logf("delete tokens for user %s", userJohn.Email)
	{
		err := rfrTokenRps.DeleteByUserID(ctx, userJohn.ID)
		require.NoError(err, "failed to delete token")
	}

	t.Logf("verify that tokens are not present in database")
	{
		johnDBTokens, err := rfrTokenRps.FindTokensByUserID(ctx, userJohn.ID)
		require.NoError(err, "failed to read tokens")
		expected := 0
		actual := len(johnDBTokens)
		require.Equal(expected, actual, "user %s tokens where deleted, but got %d tokens", userJohn.Email, actual)
	}

	t.Logf("find user %s single token", userHenry.Email)
	{
		henryDBToken, err := rfrTokenRps.FindByID(ctx, henryToken.ID)
		require.NoError(err, "failed to read token")
		require.NotNil(henryDBToken, "token was created for user %s, but not found in postgres", userHenry.Email)
	}

	t.Logf("delete user %s token", userHenry.Email)
	{
		err := rfrTokenRps.DeleteByID(ctx, henryToken.ID)
		require.NoError(err, "failed to delete token")
	}

	t.Logf("verify user %s token was deleted", userHenry.Email)
	{
		henryDBToken, err := rfrTokenRps.FindByID(ctx, henryToken.ID)
		require.NoError(err, "failed to read token")
		require.Nil(henryDBToken, "token for user %s was deleted, but still present in database", userHenry.Email)
	}
}

func (s *repositoryTestSuite) TestPostgresCustomerRps() {
	s.T().Log("running tests for postgres")
	s.testCustomerRps(NewPostgresCustomerRepository(s.pgPool))
}

func (s *repositoryTestSuite) TestMongoCustomerRps() {
	s.T().Log("running tests for mongo")
	s.testCustomerRps(NewMongoCustomerRepository(s.mongoClient))
}

func (s *repositoryTestSuite) testCustomerRps(customerRps CustomerRepository) {
	t := s.T()
	require := s.Require()

	ctx, cancel := context.WithTimeout(context.Background(), testCtxTimeout)
	defer cancel()

	middleName := "Ben"

	customers := []*model.Customer{
		{
			ID:         "53b9062b-0f45-4671-8c01-52fce0d8c750",
			FirstName:  "John",
			LastName:   "Norman",
			MiddleName: nil,
			Email:      "johnnorman@somemal.com",
			Importance: model.ImportanceLow,
			Inactive:   false,
		},
		{
			ID:         "48fa2e4f-7937-4257-ac61-a42ef9f45f69",
			FirstName:  "Albert",
			LastName:   "Peers",
			MiddleName: &middleName,
			Email:      "albertpeers@somemal.com",
			Importance: model.ImportanceMedium,
			Inactive:   false,
		},
		{
			ID:         "3b9974de-ed71-4a5d-9121-42213e526234",
			FirstName:  "Andrew",
			LastName:   "Wallet",
			MiddleName: nil,
			Email:      "andrewallet@somemal.com",
			Importance: model.ImportanceHigh,
			Inactive:   true,
		},
		{
			ID:         "f917ab49-55f3-4b92-8abd-1f1124630cd9",
			FirstName:  "Oliver",
			LastName:   "Jefferson",
			MiddleName: &middleName,
			Email:      "oliverjeff@somemal.com",
			Importance: model.ImportanceCritical,
			Inactive:   true,
		},
	}

	customerJohn := customers[0]

	customerJohnUpd := &model.Customer{
		ID:         customerJohn.ID,
		FirstName:  customerJohn.FirstName,
		LastName:   customerJohn.LastName,
		MiddleName: nil,
		Email:      "newjohn@somemail.com",
		Importance: model.ImportanceCritical,
		Inactive:   true,
	}

	t.Logf("create %d customers", len(customers))
	{
		for _, c := range customers {
			err := customerRps.Create(ctx, c)
			require.NoError(err, "failed to create customer")
		}
	}

	t.Logf("verify %d customers in database", len(customers))
	{
		dbCustomers, err := customerRps.FindAll(ctx)
		require.NoError(err, "failed to read customers")
		expected := len(customers)
		actual := len(dbCustomers)
		require.Equal(expected, actual, "%d customers were created, but got %d", expected, actual)
	}

	t.Logf("find customer by id %s", customerJohn.ID)
	{
		dbCustomer, err := customerRps.FindByID(ctx, customerJohn.ID)
		require.NoError(err, "failed to read customer")
		require.NotNil(dbCustomer, "customer was created, but not found in database")
		require.Equal(customerJohn, dbCustomer, "customer created in database is not the same it was passed")
	}

	t.Logf("update customer %s", customerJohn.ID)
	{
		err := customerRps.Update(ctx, customerJohnUpd)
		require.NoError(err, "failed to update customer")
	}

	t.Logf("find customer by id %s and verify it is updated", customerJohn.ID)
	{
		dbCustomer, err := customerRps.FindByID(ctx, customerJohn.ID)
		require.NoError(err, "failed to read customer")
		require.NotNil(dbCustomer, "customer was created and deleted, but not found in database")
		require.Equal(customerJohnUpd, dbCustomer, "customer is in database, but wasn't updated correctly")
	}

	t.Logf("delete customer by id %s", customerJohn.ID)
	{
		err := customerRps.DeleteByID(ctx, customerJohnUpd.ID)
		require.NoError(err, "failed to delete customer")
	}

	t.Logf("verify customer %s is deleted", customerJohn.ID)
	{
		dbCustomer, err := customerRps.FindByID(ctx, customerJohnUpd.ID)
		require.NoError(err, "failed to read customer by id")
		require.Nil(dbCustomer, "customer was deleted, but still present in database")
	}

	t.Logf("verify %d entries left", len(customers)-1)
	{
		dbCustomers, err := customerRps.FindAll(ctx)
		require.NoError(err, "failed to read customers")
		expected := len(customers) - 1
		actual := len(dbCustomers)
		require.Equal(expected, actual, "there must be %d customers in database, but got %d", expected, actual)
	}
}

// start repository test suite
func TestRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(repositoryTestSuite))
}
