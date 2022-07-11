package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/handlers"
	"github.com/umalmyha/customers/internal/middleware"
	"github.com/umalmyha/customers/internal/model/auth"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const Port = 3000
const ShutdownTimeout = 10 * time.Second
const ServerStartupTimeout = 10 * time.Second

func main() {
	cfg, err := config.Build()
	if err != nil {
		log.Fatalf(err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), ServerStartupTimeout)
	defer cancel()

	pgPool, err := postgresql(ctx, cfg.PostgresCfg)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer pgPool.Close()

	mongoClient, err := mongodb(ctx, cfg.MongoCfg)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer func() {
		if err = mongoClient.Disconnect(ctx); err != nil {
			log.Fatalf(err.Error())
		}
	}()

	start(pgPool, mongoClient, cfg.AuthCfg)
}

func start(pgPool *pgxpool.Pool, mongoClient *mongo.Client, authCfg config.AuthCfg) {
	app := app(pgPool, mongoClient, authCfg)

	shutdownCh := make(chan os.Signal, 1)
	errorCh := make(chan error, 1)
	signal.Notify(shutdownCh, os.Interrupt)

	go func() {
		errorCh <- app.Start(fmt.Sprintf(":%d", Port))
	}()

	select {
	case <-shutdownCh:
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		app.Logger.Infof("shutdown signal has been sent, stopping the server...")
		if err := app.Shutdown(ctx); err != nil {
			app.Logger.Fatalf("failed to stop server gracefully - %s", err)
		}
	case err := <-errorCh:
		if !errors.Is(err, http.ErrServerClosed) {
			app.Logger.Fatalf("shutting down the server, unexpected error occurred - %s", err)
		}
	}
}

func mongodb(ctx context.Context, cfg config.MongoCfg) (*mongo.Client, error) {
	uri := fmt.Sprintf("mongodb://%s:%s@mongo-customers:27017/?maxPoolSize=%d", cfg.User, cfg.Password, cfg.MaxPoolSize)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}
	return client, nil
}

func postgresql(ctx context.Context, cfg config.PostgresCfg) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf("user=%s password=%s host=pg-customers port=5432 dbname=%s sslmode=%s pool_max_conns=%d", cfg.User, cfg.Password, cfg.Database, cfg.SslMode, cfg.PoolMaxConn)

	pool, err := pgxpool.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to establish connection to db - %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("didn't get response from database after sending ping request - %w", err)
	}
	return pool, nil
}

func app(pgPool *pgxpool.Pool, mongoClient *mongo.Client, authCfg config.AuthCfg) *echo.Echo {
	e := echo.New()

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		c.Logger().Error(err.Error())
		e.DefaultHTTPErrorHandler(err, c)
	}

	// Transactors
	pgxTransactor := transactor.NewPgxTransactor(pgPool)
	pgxTxExecutor := transactor.NewPgxWithinTransactionExecutor(pgPool)

	// Extra functionality
	jwtIssuer := auth.NewJwtIssuer(authCfg.JwtCfg.Issuer, authCfg.JwtCfg.SigningMethod, authCfg.JwtCfg.TimeToLive, authCfg.JwtCfg.PrivateKey)
	jwtValidator := auth.NewJwtValidator(authCfg.JwtCfg.SigningMethod, authCfg.JwtCfg.PublicKey)

	// Middleware
	authorizeMw := middleware.Authorize(jwtValidator)

	// Repositories
	userRps := repository.NewPostgresUserRepository(pgxTxExecutor)
	rfrTokenRps := repository.NewPostgresRefreshTokenRepository(pgxTxExecutor)
	pgCustomerRps := repository.NewPostgresCustomerRepository(pgPool)
	mongoCustomerRps := repository.NewMongoCustomerRepository(mongoClient)

	// Services
	authSvc := service.NewAuthService(jwtIssuer, authCfg.RefreshTokenCfg, pgxTransactor, userRps, rfrTokenRps)
	customerSvcV1 := service.NewCustomerService(pgCustomerRps)
	customerSvcV2 := service.NewCustomerService(mongoCustomerRps)

	// Handlers
	authHandler := handlers.NewAuthHandler(authSvc)
	customerHandlerV1 := handlers.NewCustomerHandler(customerSvcV1)
	customerHandlerV2 := handlers.NewCustomerHandler(customerSvcV2)

	// API routes
	api := e.Group("/api")
	{
		// auth
		authApi := api.Group("/auth")
		{
			authApi.POST("/signup", authHandler.Signup)
			authApi.POST("/login", authHandler.Login)
			authApi.POST("/logout", authHandler.Logout)
			authApi.POST("/refresh", authHandler.Refresh)
		}

		// customers v1
		customersApiV1 := api.Group("/v1/customers", authorizeMw)
		{
			customersApiV1.GET("", customerHandlerV1.GetAll)
			customersApiV1.GET("/:id", customerHandlerV1.Get)
			customersApiV1.POST("", customerHandlerV1.Post)
			customersApiV1.PUT("/:id", customerHandlerV1.Put)
			customersApiV1.DELETE("/:id", customerHandlerV1.DeleteById)
		}

		// customers v2
		customersApiV2 := api.Group("/v2/customers", authorizeMw)
		{
			customersApiV2.GET("", customerHandlerV2.GetAll)
			customersApiV2.GET("/:id", customerHandlerV2.Get)
			customersApiV2.POST("", customerHandlerV2.Post)
			customersApiV2.PUT("/:id", customerHandlerV2.Put)
			customersApiV2.DELETE("/:id", customerHandlerV2.DeleteById)
		}
	}

	return e
}
