package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/labstack/gommon/log"
	"github.com/ory/dockertest/v3"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	connectionTimeout = 3 * time.Second
)

const (
	pgContainerName = "pg-test-customers"
	pgPort          = "5432"
	pgTestUser      = "test"
	pgTestPassword  = "test"
	pgTestDB        = "customers"
)

const (
	mongoContainerName = "mongo-test-customers"
	mongoPort          = "27017"
	mongoTestUser      = "test"
	mongoTestPassword  = "test"
)

var pgPool *pgxpool.Pool
var mongoClient *mongo.Client

func TestMain(m *testing.M) {
	// build docker pool
	dockerPool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("failed to create pool - %v", err)
	}

	if err := dockerPool.Client.Ping(); err != nil {
		log.Fatalf("failed to connect to docker - %v", err)
	}

	// create network for containers
	network, err := dockerPool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: "customers-test-net"})
	if err != nil {
		log.Fatalf("failed to create network - %v", err)
	}

	// start postgres
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
	if err != nil {
		log.Fatalf("failed to start postgresql - %v", err)
	}

	// run migrations
	flywayCmd := []string{
		fmt.Sprintf("-url=jdbc:postgresql://%s:%s/%s", pgContainerName, pgPort, pgTestDB),
		fmt.Sprintf("-user=%s", pgTestUser),
		fmt.Sprintf("-password=%s", pgTestPassword),
		"-connectRetries=5",
		"migrate",
	}

	migrationsPath, err := filepath.Abs("../../migrations")
	if err != nil {
		log.Fatalf("failed to find migrations path - %v", err)
	}

	flywayMounts := []string{
		fmt.Sprintf("%s:/flyway/sql", migrationsPath),
	}

	flyway, err := dockerPool.RunWithOptions(&dockertest.RunOptions{
		Repository: "flyway/flyway",
		Tag:        "latest",
		NetworkID:  network.ID,
		Cmd:        flywayCmd,
		Mounts:     flywayMounts,
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
	})
	if err != nil {
		log.Fatalf("failed to start flyway migrations - %v", err)
	}

	// waiting for flyway container to be destroyed
	err = dockerPool.Retry(func() error {
		if _, ok := dockerPool.ContainerByName(flyway.Container.Name); ok {
			return errors.New("flyway migrations are still in progress")
		}
		return nil
	})
	if err != nil {
		log.Fatalf("failed to await flyway migrations - %v", err)
	}

	// connect to postgres
	pgUri := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", pgTestUser, pgTestPassword, pgPort, pgTestDB)
	err = dockerPool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
		defer cancel()

		var err error
		pgPool, err = pgxpool.Connect(ctx, pgUri)
		if err != nil {
			return err
		}
		return pgPool.Ping(ctx)
	})
	if err != nil {
		log.Fatalf("failed to establish connection to postgresql - %v", err)
	}

	// start mongo
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
	if err != nil {
		log.Fatalf("failed to start mongodb - %v", err)
	}

	// connect to mongo
	mongoUri := fmt.Sprintf("mongodb://%s:%s@localhost:%s/?maxPoolSize=100", mongoTestUser, mongoTestPassword, mongoPort)
	err = dockerPool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
		defer cancel()

		var err error
		mongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoUri))
		if err != nil {
			return err
		}
		return mongoClient.Ping(ctx, readpref.Primary())
	})
	if err != nil {
		log.Fatalf("failed to establish connection to mongodb - %v", err)
	}

	// start tests
	code := m.Run()

	// purge postgresql
	if err := dockerPool.Purge(postgres); err != nil {
		log.Fatalf("failed to purge postgresql - %v", err)
	}

	// purge mongodb
	if err := dockerPool.Purge(mongodb); err != nil {
		log.Fatalf("failed to purge mongodb - %v", err)
	}

	// remove network
	if err := dockerPool.Client.RemoveNetwork(network.ID); err != nil {
		log.Fatalf("failed to remove network - %v", err)
	}

	os.Exit(code)
}

func TestUserRps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userRps := NewPostgresUserRepository(transactor.NewPgxWithinTransactionExecutor(pgPool))

	u := &model.User{
		ID:           "f9771714-df35-4186-b1f1-57fba3e5d3f2",
		Email:        "customer1@somemail.com",
		PasswordHash: "f929cb58673be0a35fcb22ad7f147bd1",
	}

	t.Log("create user")
	{
		err := userRps.Create(ctx, u)
		require.NoError(t, err, "failed to create user")
	}

	t.Log("find user by id")
	{
		dbUser, err := userRps.FindByID(ctx, u.ID)
		require.NoError(t, err, "failed to read user by id")
		require.NotNil(t, dbUser, "user was created recently but not found by id")
	}

	t.Log("find user by email")
	{
		dbUser, err := userRps.FindByEmail(ctx, u.Email)
		require.NoError(t, err, "failed to read user by email")
		require.NotNil(t, dbUser, "user was created recently but not found by email")
	}

	t.Log("create user duplicate")
	{
		err := userRps.Create(ctx, u)
		require.Error(t, err, "aimed to create user duplicate but no error raised")
	}
}

func TestRefreshTokenRps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	expiresIn := 3000
	fingerprint := "b86de171-7481-4b57-a012-765e6e34e2c2"
	createdAt := time.Now().UTC()

	userRps := NewPostgresUserRepository(transactor.NewPgxWithinTransactionExecutor(pgPool))
	rfrTokenRps := NewPostgresRefreshTokenRepository(transactor.NewPgxWithinTransactionExecutor(pgPool))

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
		require.NoError(t, err, "failed to create user %s", userJohn.Email)

		err = userRps.Create(ctx, userHenry)
		require.NoError(t, err, "failed to create user %s", userHenry.Email)
	}

	t.Logf("create %d tokens", len(refreshTokens))
	{
		for _, tkn := range refreshTokens {
			err := rfrTokenRps.Create(ctx, tkn)
			require.NoError(t, err, "failed to create token %s", tkn.ID)
		}
	}

	t.Logf("find tokens for user %s", userJohn.Email)
	{
		johnDBTokens, err := rfrTokenRps.FindTokensByUserID(ctx, userJohn.ID)
		require.NoError(t, err, "failed to read tokens")
		expected := 2
		actual := len(johnDBTokens)
		require.Equal(t, expected, actual, "%d tokens where created for user %s, got %d", expected, userJohn.Email, actual)
	}

	t.Logf("delete tokens for user %s", userJohn.Email)
	{
		err := rfrTokenRps.DeleteByUserID(ctx, userJohn.ID)
		require.NoError(t, err, "failed to delete token")
	}

	t.Logf("verify that tokens are not present in database")
	{
		johnDBTokens, err := rfrTokenRps.FindTokensByUserID(ctx, userJohn.ID)
		require.NoError(t, err, "failed to read tokens")
		expected := 0
		actual := len(johnDBTokens)
		require.Equal(t, expected, actual, "user %s tokens where deleted, but got %d tokens", userJohn.Email, actual)
	}

	t.Logf("find user %s single token", userHenry.Email)
	{
		henryDBToken, err := rfrTokenRps.FindByID(ctx, henryToken.ID)
		require.NoError(t, err, "failed to read token")
		require.NotNil(t, henryDBToken, "token was created for user %s, but not found in postgres", userHenry.Email)
	}

	t.Logf("delete user %s token", userHenry.Email)
	{
		err := rfrTokenRps.DeleteByID(ctx, henryToken.ID)
		require.NoError(t, err, "failed to delete token")
	}

	t.Logf("verify user %s token was deleted", userHenry.Email)
	{
		henryDBToken, err := rfrTokenRps.FindByID(ctx, henryToken.ID)
		require.NoError(t, err, "failed to read token")
		require.Nil(t, henryDBToken, "token for user %s was deleted, but still present in database", userHenry.Email)
	}
}

func TestPostgresCustomerRps(t *testing.T) {
	customerRps := NewPostgresCustomerRepository(pgPool)
	t.Log("running tests for postgres")
	testCustomerRps(t, customerRps)
}

func TestMongoCustomerRps(t *testing.T) {
	customerRps := NewMongoCustomerRepository(mongoClient)
	t.Log("running tests for mongo")
	testCustomerRps(t, customerRps)
}

func testCustomerRps(t *testing.T, customerRps CustomerRepository) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	t.Log("create 4 customers")
	{
		for _, c := range customers {
			err := customerRps.Create(ctx, c)
			require.NoError(t, err, "failed to create customer")
		}
	}

	t.Logf("verify %d customers in database", len(customers))
	{
		dbCustomers, err := customerRps.FindAll(ctx)
		require.NoError(t, err, "failed to read customers")
		expected := len(customers)
		actual := len(dbCustomers)
		require.Equal(t, expected, actual, "%d customers were created, but got %d", expected, actual)
	}

	t.Logf("find customer by id %s", customerJohn.ID)
	{
		dbCustomer, err := customerRps.FindByID(ctx, customerJohn.ID)
		require.NoError(t, err, "failed to read customer")
		require.NotNil(t, dbCustomer, "customer was created, but not found in database")
		require.Equal(t, customerJohn, dbCustomer, "customer created in database is not the same it was passed")
	}

	t.Logf("update customer %s", customerJohn.ID)
	{
		err := customerRps.Update(ctx, customerJohnUpd)
		require.NoError(t, err, "failed to update customer")
	}

	t.Logf("find customer by id %s and verify it is updated", customerJohn.ID)
	{
		dbCustomer, err := customerRps.FindByID(ctx, customerJohn.ID)
		require.NoError(t, err, "failed to read customer")
		require.NotNil(t, dbCustomer, "customer was created and deleted, but not found in database")
		require.Equal(t, customerJohnUpd, dbCustomer, "customer is in database, but wasn't updated correctly")
	}

	t.Logf("delete customer by id %s", customerJohn.ID)
	{
		err := customerRps.DeleteByID(ctx, customerJohnUpd.ID)
		require.NoError(t, err, "failed to delete customer")
	}

	t.Logf("verify customer %s is deleted", customerJohn.ID)
	{
		dbCustomer, err := customerRps.FindByID(ctx, customerJohnUpd.ID)
		require.NoError(t, err, "failed to read customer by id")
		require.Nil(t, dbCustomer, "customer was deleted, but still present in database")
	}

	t.Logf("verify %d entries left", len(customers)-1)
	{
		dbCustomers, err := customerRps.FindAll(ctx)
		require.NoError(t, err, "failed to read customers")
		expected := len(customers) - 1
		actual := len(dbCustomers)
		require.Equal(t, expected, actual, "there must be %d customers in database, but got %d", expected, actual)
	}
}
