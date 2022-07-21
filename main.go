package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTrans "github.com/go-playground/validator/v10/translations/en"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/handlers"
	"github.com/umalmyha/customers/internal/middleware"
	"github.com/umalmyha/customers/internal/model/auth"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/internal/validation"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"time"
)

const Port = 3000
const ShutdownTimeout = 10 * time.Second
const ServerStartupTimeout = 10 * time.Second

func main() {
	logger := logger()

	cfg, err := config.Build()
	if err != nil {
		logger.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), ServerStartupTimeout)
	defer cancel()

	pgPool, err := postgresql(ctx, cfg.PostgresConnString)
	if err != nil {
		logger.Fatal(err)
	}
	defer pgPool.Close()

	mongoClient, err := mongodb(ctx, cfg.MongoConnString)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err = mongoClient.Disconnect(ctx); err != nil {
			logger.Fatal(err)
		}
	}()

	start(pgPool, mongoClient, logger, cfg.AuthCfg)
}

func start(pgPool *pgxpool.Pool, mongoClient *mongo.Client, logger logrus.FieldLogger, authCfg config.AuthCfg) {
	app := app(pgPool, mongoClient, logger, authCfg)

	shutdownCh := make(chan os.Signal, 1)
	errorCh := make(chan error, 1)
	signal.Notify(shutdownCh, os.Interrupt)

	go func() {
		logger.Infof("Starting server at port :%d", Port)
		errorCh <- app.Start(fmt.Sprintf(":%d", Port))
	}()

	select {
	case <-shutdownCh:
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		logger.Infof("shutdown signal has been sent, stopping the server...")
		if err := app.Shutdown(ctx); err != nil {
			logger.Errorf("failed to stop server gracefully - %v", err)
		}
	case err := <-errorCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Errorf("shutting down the server because of unexpected error - %v", err)
		}
	}
}

func mongodb(ctx context.Context, uri string) (*mongo.Client, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}
	return client, nil
}

func postgresql(ctx context.Context, uri string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.Connect(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to establish connection to db - %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("didn't get response from database after sending ping request - %w", err)
	}
	return pool, nil
}

func logger() logrus.FieldLogger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetReportCaller(true)
	logger.SetOutput(os.Stdout)
	return logger
}

func app(pgPool *pgxpool.Pool, mongoClient *mongo.Client, logger logrus.FieldLogger, authCfg config.AuthCfg) *echo.Echo {
	e := echo.New()

	validator, err := echoValidator()
	if err != nil {
		logger.Fatal(err)
	}

	e.Validator = validator

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		logger.Errorf("error occurred during request processing - %v", err)

		var pldErr *validation.PayloadError
		if errors.As(err, &pldErr) {
			err = c.JSON(http.StatusBadRequest, pldErr)
			if err == nil {
				return
			}
		}

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
	authSvc := service.NewAuthService(jwtIssuer, authCfg.RefreshTokenCfg, pgxTransactor, userRps, rfrTokenRps, logger)
	customerSvcV1 := service.NewCustomerService(pgCustomerRps, logger)
	customerSvcV2 := service.NewCustomerService(mongoCustomerRps, logger)

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

func echoValidator() (echo.Validator, error) {
	valid := validator.New()

	// store json tag fields, so can be handled on UI properly in struct PayloadErr -> field Field
	valid.RegisterTagNameFunc(func(field reflect.StructField) string {
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			return field.Name
		}
		return jsonName
	})

	en := en.New()
	unvTrans := ut.New(en, en)
	trans, ok := unvTrans.GetTranslator("en")
	if !ok {
		return nil, errors.New("failed to find translator for en locale")
	}

	// register default translations
	if err := enTrans.RegisterDefaultTranslations(valid, trans); err != nil {
		return nil, fmt.Errorf("failed to register en translations - %w", err)
	}

	return validation.Echo(valid, trans), nil
}
