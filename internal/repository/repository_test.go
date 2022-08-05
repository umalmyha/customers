package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/pkg/db/transactor"
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
	pgContainerName = "pg-customers"
	pgPort          = "5432"
	pgTestUser      = "test"
	pgTestPassword  = "test"
	pgTestDB        = "customers"
)

var pgPool *pgxpool.Pool
var mongoClient *mongo.Client

func TestMain(m *testing.M) {
	dockerPool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("failed to create pool - %v", err)
	}

	if err := dockerPool.Client.Ping(); err != nil {
		log.Fatalf("failed to connect to docker - %v", err)
	}

	network, err := dockerPool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: "customers-net"})
	if err != nil {
		log.Fatalf("failed to create network - %v", err)
	}

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

	// run migrations
	cmd := []string{
		fmt.Sprintf("-url=jdbc:postgresql://%s:%s/%s", pgContainerName, pgPort, pgTestDB),
		fmt.Sprintf("-user=%s", pgTestUser),
		fmt.Sprintf("-password=%s", pgTestPassword),
		"-connectRetries=5",
		"migrate",
	}

	migrationsPath, err := filepath.Abs("../../migrations")
	mounts := []string{
		fmt.Sprintf("%s:/flyway/sql", migrationsPath),
	}

	flyway, err := dockerPool.RunWithOptions(&dockertest.RunOptions{
		Repository: "flyway/flyway",
		Tag:        "latest",
		NetworkID:  network.ID,
		Cmd:        cmd,
		Mounts:     mounts,
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

	// start tests
	code := m.Run()

	// purge postgresql
	if err := dockerPool.Purge(postgres); err != nil {
		log.Fatalf("failed to purge postgresql - %v", err)
	}

	// remove network
	if err := dockerPool.Client.RemoveNetwork(network.ID); err != nil {
		log.Fatalf("failed to remove network - %v", err)
	}

	os.Exit(code)
}

func TestUserRpsEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userRps := NewPostgresUserRepository(transactor.NewPgxWithinTransactionExecutor(pgPool))

	u := &model.User{
		ID:           uuid.NewString(),
		Email:        "customer1@somemail.com",
		PasswordHash: "f929cb58673be0a35fcb22ad7f147bd1",
	}

	t.Log("create user")
	{
		err := userRps.Create(ctx, u)
		require.NoError(t, err, "failed to access postgres for user creation")
	}

	t.Log("find user by id")
	{
		dbUser, err := userRps.FindByID(ctx, u.ID)
		require.NoError(t, err, "failed to access postgres to find user by id")
		require.NotNil(t, dbUser, "user was created recently but not found by id")
	}

	t.Log("find user by email")
	{
		dbUser, err := userRps.FindByEmail(ctx, u.Email)
		require.NoError(t, err, "failed to access postgres to find user by email")
		require.NotNil(t, dbUser, "user was created recently but not found by email")
	}

	t.Log("create user duplicate")
	{
		err := userRps.Create(ctx, u)
		require.Error(t, err, "aimed to create user duplicate but no error raised")
	}
}

func TestRefreshTokenEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	expiresIn := 3000
	fingerprint := "db33ce19-b7d7-4bc2-b8ab-a4cdbe88a911"
	createdAt := time.Now().UTC()

	userRps := NewPostgresUserRepository(transactor.NewPgxWithinTransactionExecutor(pgPool))
	rfrTokenRps := NewPostgresRefreshTokenRepository(transactor.NewPgxWithinTransactionExecutor(pgPool))

	userJohn := &model.User{
		ID:           uuid.NewString(),
		Email:        "john@somemail.com",
		PasswordHash: "7c9fb260749f6d1cf54530450ac97f72",
	}

	userHenry := &model.User{
		ID:           uuid.NewString(),
		Email:        "henry@somemail.com",
		PasswordHash: "966ac2a7543413f3368a2fc3ca889f98",
	}

	// john has 2 tokens and henry has 1 token
	refreshTokens := []*model.RefreshToken{
		{
			ID:          uuid.NewString(),
			UserID:      userJohn.ID,
			Fingerprint: fingerprint,
			ExpiresIn:   expiresIn,
			CreatedAt:   createdAt,
		},
		{
			ID:          uuid.NewString(),
			UserID:      userJohn.ID,
			Fingerprint: fingerprint,
			ExpiresIn:   expiresIn,
			CreatedAt:   createdAt,
		},
		{
			ID:          uuid.NewString(),
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
		require.NoError(t, err, "failed to access postgres for user creation")

		err = userRps.Create(ctx, userHenry)
		require.NoError(t, err, "failed to access postgres for user creation")
	}

	t.Log("create several tokens")
	{
		for _, tkn := range refreshTokens {
			err := rfrTokenRps.Create(ctx, tkn)
			require.NoError(t, err, "failed to access postgres for token creation")
		}
	}

	t.Logf("find tokens for user %s", userJohn.Email)
	{
		johnDBTokens, err := rfrTokenRps.FindTokensByUserID(ctx, userJohn.ID)
		require.NoError(t, err, "failed to access postgres to read tokens")
		require.Equal(t, 2, len(johnDBTokens), "2 tokens where created for user %s, got %d", userJohn.Email, len(johnDBTokens))
	}

	t.Logf("delete tokens for user %s", userJohn.Email)
	{
		err := rfrTokenRps.DeleteByUserID(ctx, userJohn.ID)
		require.NoError(t, err, "failed to access postgres to delete token")
	}

	t.Logf("verify tokens are not present in postgres")
	{
		johnDBTokens, err := rfrTokenRps.FindTokensByUserID(ctx, userJohn.ID)
		require.NoError(t, err, "failed to access postgres to read tokens")
		require.Equal(t, 0, len(johnDBTokens), "user %s tokens where deleted, but got %d tokens", userJohn.Email, len(johnDBTokens))
	}

	t.Logf("find user %s single token", userHenry.Email)
	{
		henryDBToken, err := rfrTokenRps.FindByID(ctx, henryToken.ID)
		require.NoError(t, err, "failed to access postgres to read token")
		require.NotNil(t, henryDBToken, "token was created for user %s, but not found in postgres", userHenry.Email)
	}

	t.Logf("delete user %s token", userHenry.Email)
	{
		err := rfrTokenRps.DeleteByID(ctx, henryToken.ID)
		require.NoError(t, err, "failed to access postgres to delete token")
	}

	t.Logf("verify user %s token was deleted", userHenry.Email)
	{
		henryDBToken, err := rfrTokenRps.FindByID(ctx, henryToken.ID)
		require.NoError(t, err, "failed to access postgres to read token")
		require.Nil(t, henryDBToken, "token for user %s was deleted, but still present in postgres", userHenry.Email)
	}
}
