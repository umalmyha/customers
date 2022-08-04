package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTrans "github.com/go-playground/validator/v10/translations/en"
	"github.com/go-redis/redis/v9"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/labstack/echo/v4"
	echoMw "github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
	echoSwagger "github.com/swaggo/echo-swagger"
	_ "github.com/umalmyha/customers/docs"
	"github.com/umalmyha/customers/internal/cache"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/handlers"
	"github.com/umalmyha/customers/internal/interceptors"
	"github.com/umalmyha/customers/internal/middleware"
	"github.com/umalmyha/customers/internal/model/auth"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/umalmyha/customers/internal/proto"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/internal/validation"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"github.com/vmihailenco/msgpack/v5"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"time"
)

const HttpPort = 3000
const GrpcPort = 3010
const ShutdownTimeout = 10 * time.Second
const ServerStartupTimeout = 10 * time.Second

// @title Customers API
// @version 1.0
// @description API allows to perform CRUD on customer entity

// @contact.name Uladzislau Malmyha
// @contact.url https://github.com/umalmyha/customers/issues
// @contact.email uladzislau.malmyha@gmail.com

// @host localhost:3000
// @BasePath /

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
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

	redisClient, err := redisClient(ctx, cfg.RedisCfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer redisClient.Close()

	mongoClient, err := mongodb(ctx, cfg.MongoConnString)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err = mongoClient.Disconnect(ctx); err != nil {
			logger.Fatal(err)
		}
	}()

	start(pgPool, mongoClient, redisClient, logger, cfg.AuthCfg)
}

func start(pgPool *pgxpool.Pool, mongoClient *mongo.Client, redisClient *redis.Client, logger logrus.FieldLogger, authCfg config.AuthCfg) {
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

	// caches
	redisCustomerCache := cache.NewRedisCustomerCache(redisClient)
	inMemoryCustomerCache := cache.NewInMemoryCache()
	redisStreamCustomerCache := cache.NewRedisStreamCustomerCache(redisClient, inMemoryCustomerCache)

	// Repositories
	userRps := repository.NewPostgresUserRepository(pgxTxExecutor)
	rfrTokenRps := repository.NewPostgresRefreshTokenRepository(pgxTxExecutor)
	pgCustomerRps := repository.NewPostgresCustomerRepository(pgPool)
	mongoCustomerRps := repository.NewMongoCustomerRepository(mongoClient)
	pgCachedCustomerRps := repository.NewRedisCachedCustomerRepository(logger, redisCustomerCache, pgCustomerRps)
	mongoCachedCustomerRps := repository.NewRedisCachedCustomerRepository(logger, redisStreamCustomerCache, mongoCustomerRps)

	// Services
	authSvc := service.NewAuthService(jwtIssuer, authCfg.RefreshTokenCfg, pgxTransactor, userRps, rfrTokenRps, logger)
	customerSvcV1 := service.NewCustomerService(pgCachedCustomerRps, logger)
	customerSvcV2 := service.NewCustomerService(mongoCachedCustomerRps, logger)

	// HTTP Handlers
	authHttpHandler := handlers.NewAuthHttpHandler(authSvc)
	customerHttpHandlerV1 := handlers.NewCustomerHttpHandler(customerSvcV1)
	customerHttpHandlerV2 := handlers.NewCustomerHttpHandler(customerSvcV2)
	imageHandler := handlers.NewImageHandler()

	// gRPC Handlers
	authGrpcHandler := handlers.NewAuthGrpcHandler(authSvc)
	customerGrpcHandler := handlers.NewCustomerGrpcHandler(customerSvcV1)

	// interceptors
	authInterceptor := interceptors.AuthUnaryInterceptor(jwtValidator, interceptors.UnaryApplicableForService("CustomerService"))
	validatorInterceptor := interceptors.ValidatorUnaryInterceptor(true)
	errorInterceptor := interceptors.ErrorUnaryInterceptor(logger)

	images := e.Group("/images")
	{
		images.POST("/upload", imageHandler.Upload)
		images.GET("/:name/download", imageHandler.Download)
		images.Use(echoMw.StaticWithConfig(echoMw.StaticConfig{
			Root:   "images",
			Browse: true,
		}))
	}

	// API routes
	api := e.Group("/api")
	{
		// auth
		authApi := api.Group("/auth")
		{
			authApi.POST("/signup", authHttpHandler.Signup)
			authApi.POST("/login", authHttpHandler.Login)
			authApi.POST("/logout", authHttpHandler.Logout)
			authApi.POST("/refresh", authHttpHandler.Refresh)
		}

		// customers v1
		customersApiV1 := api.Group("/v1/customers", authorizeMw)
		{
			customersApiV1.GET("", customerHttpHandlerV1.GetAll)
			customersApiV1.GET("/:id", customerHttpHandlerV1.Get)
			customersApiV1.POST("", customerHttpHandlerV1.Post)
			customersApiV1.PUT("/:id", customerHttpHandlerV1.Put)
			customersApiV1.DELETE("/:id", customerHttpHandlerV1.DeleteById)
		}

		// customers v2
		customersApiV2 := api.Group("/v2/customers", authorizeMw)
		{
			customersApiV2.GET("", customerHttpHandlerV2.GetAll)
			customersApiV2.GET("/:id", customerHttpHandlerV2.Get)
			customersApiV2.POST("", customerHttpHandlerV2.Post)
			customersApiV2.PUT("/:id", customerHttpHandlerV2.Put)
			customersApiV2.DELETE("/:id", customerHttpHandlerV2.DeleteById)
		}
	}

	e.GET("/swagger/*", echoSwagger.WrapHandler)

	shutdownCh := make(chan os.Signal, 1)
	errorCh := make(chan error, 1)
	signal.Notify(shutdownCh, os.Interrupt)

	// start HTTP server
	go func() {
		logger.Infof("Starting HTTP server at port :%d", HttpPort)
		if err := e.Start(fmt.Sprintf(":%d", HttpPort)); err != nil {
			logger.Error("HTTP server raised error")
			errorCh <- err
		}
	}()

	// start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", GrpcPort))
	if err != nil {
		logger.Fatal(err)
	}

	grpcSvc := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			authInterceptor,
			validatorInterceptor,
			errorInterceptor,
		),
	)

	proto.RegisterAuthServiceServer(grpcSvc, authGrpcHandler)
	proto.RegisterCustomerServiceServer(grpcSvc, customerGrpcHandler)

	go func() {
		logger.Infof("Starting gRPC server at port :%d", GrpcPort)
		if err := grpcSvc.Serve(lis); err != nil {
			logger.Error("gRPC server raised error")
			errorCh <- err
		}
	}()

	// start redis steam listen loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go readCustomersStream(ctx, redisClient, logger, inMemoryCustomerCache)

	select {
	case <-shutdownCh:
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		logger.Info("shutdown signal has been sent")
		logger.Info("stopping the HTTP server...")
		if err := e.Shutdown(ctx); err != nil {
			logger.Errorf("failed to stop server gracefully - %v", err)
		}

		logger.Info("stopping the gRPC server...")
		grpcSvc.Stop()
	case err := <-errorCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Errorf("shutting down the servers because of unexpected error - %v", err)
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

func redisClient(ctx context.Context, cfg config.RedisCfg) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:       cfg.Addr,
		Password:   cfg.Password,
		DB:         cfg.Db,
		MaxRetries: cfg.MaxRetries,
		PoolSize:   cfg.PoolSize,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("didn't get response from redis after sending ping request - %w", err)
	}
	return client, nil
}

func logger() logrus.FieldLogger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetReportCaller(true)
	logger.SetOutput(os.Stdout)
	return logger
}

func echoValidator() (echo.Validator, error) {
	validator := validator.New()

	// store json tag fields, so can be handled on UI properly in struct PayloadErr -> field Field
	validator.RegisterTagNameFunc(func(field reflect.StructField) string {
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			return field.Name
		}
		return jsonName
	})

	en := en.New()
	unvTranslator := ut.New(en, en)
	translator, ok := unvTranslator.GetTranslator("en")
	if !ok {
		return nil, errors.New("failed to find translator for en locale")
	}

	// register default translations
	if err := enTrans.RegisterDefaultTranslations(validator, translator); err != nil {
		return nil, fmt.Errorf("failed to register en translations - %w", err)
	}

	return validation.Echo(validator, translator), nil
}

func readCustomersStream(ctx context.Context, client *redis.Client, logger logrus.FieldLogger, cache cache.CustomerCache) {
	const cacheWriteTimeout = 5 * time.Second
	key := "$"

	processMessage := func(m redis.XMessage) error {
		op, ok := m.Values["op"].(string)
		if !ok || op == "" {
			return errors.New("message has incorrect format - op field is missing, skipped")
		}

		value, ok := m.Values["value"].(string)
		if !ok {
			return errors.New("message has incorrect format - value field is missing, skipped")
		}

		logger.Infof("%s operation is requested", op)

		ctx, cancel := context.WithTimeout(ctx, cacheWriteTimeout)
		defer cancel()

		switch op {
		case "create":
			var c customer.Customer
			if err := msgpack.Unmarshal([]byte(value), &c); err != nil {
				return fmt.Errorf("failed to deserialize customer - %w", err)
			}

			if err := cache.Create(ctx, &c); err != nil {
				return fmt.Errorf("failed to create customer entry in cache - %w", err)
			}
		case "delete":
			if err := cache.DeleteById(ctx, value); err != nil {
				return fmt.Errorf("failed to delete customer entry from cache - %w", err)
			}
		}

		return nil
	}

	logger.Info("starting to read customers redis stream")

XRead:
	for {
		select {
		case <-ctx.Done():
			break XRead
		default:
			logger.Infof("waiting for new messages starting from %s", key)
			streams, err := client.XRead(ctx, &redis.XReadArgs{
				Streams: []string{"customers-stream", key},
				Count:   10,
				Block:   0,
			}).Result()
			if err != nil {
				logger.Errorf("error occurred on reading message from stream - %v", err)
				continue
			}

			logger.Info("messages were received")

			for _, stream := range streams {
				for _, m := range stream.Messages {
					logger.Info("number of message received = ", len(stream.Messages))

					key = m.ID
					if err := processMessage(m); err != nil {
						logger.Errorf("error occurred on message %s processing - %v", key, err)
					}
				}
			}
		}
	}
}
