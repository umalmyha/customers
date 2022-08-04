package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"time"

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
	"github.com/umalmyha/customers/internal/auth"
	"github.com/umalmyha/customers/internal/cache"
	"github.com/umalmyha/customers/internal/config"
	"github.com/umalmyha/customers/internal/handlers"
	"github.com/umalmyha/customers/internal/interceptors"
	"github.com/umalmyha/customers/internal/middleware"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/internal/service"
	"github.com/umalmyha/customers/internal/validation"
	"github.com/umalmyha/customers/pkg/db/transactor"
	"github.com/umalmyha/customers/proto"
	"github.com/vmihailenco/msgpack/v5"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"google.golang.org/grpc"
)

const httpPort = 3000
const grpcPort = 3010
const shutdownTimeout = 10 * time.Second
const serverStartupTimeout = 10 * time.Second
const readStreamMessagesMaxCount = 10
const readStreamBlockTime = 0
const cacheWriteTimeout = 5 * time.Second

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
	setupLogger()

	cfg, err := config.Build()
	if err != nil {
		logrus.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), serverStartupTimeout)
	defer cancel()

	pgPool, err := postgresql(ctx, cfg.PostgresConnString)
	if err != nil {
		logrus.Fatal(err)
	}
	defer pgPool.Close()

	redisClient, err := redisClient(ctx, cfg.RedisCfg)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		if err = redisClient.Close(); err != nil {
			logrus.Fatal(err)
		}
	}()

	mongoClient, err := mongodb(ctx, cfg.MongoConnString)
	if err != nil {
		logrus.Fatal(err)
	}
	defer func() {
		if err = mongoClient.Disconnect(ctx); err != nil {
			logrus.Fatal(err)
		}
	}()

	start(pgPool, mongoClient, redisClient, cfg.JwtCfg, cfg.RefreshTokenCfg)
}

//nolint:funlen // function contains a lot of endpoints definitions
func start(
	pgPool *pgxpool.Pool,
	mongoClient *mongo.Client,
	redisClient *redis.Client,
	jwtCfg config.JwtCfg,
	rfrTokenCfg config.RefreshTokenCfg,
) {
	e := echo.New()

	echoValidator, err := echoValidator()
	if err != nil {
		logrus.Fatal(err)
	}
	e.Validator = echoValidator

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		logrus.Errorf("error occurred during request processing - %v", err)

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
	jwtIssuer := auth.NewJwtIssuer(jwtCfg.Issuer, jwtCfg.SigningMethod, jwtCfg.TimeToLive, jwtCfg.PrivateKey)
	jwtValidator := auth.NewJwtValidator(jwtCfg.SigningMethod, jwtCfg.PublicKey)

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

	// Services
	authSvc := service.NewAuthService(jwtIssuer, rfrTokenCfg, pgxTransactor, userRps, rfrTokenRps)
	customerSvcV1 := service.NewCustomerService(pgCustomerRps, redisCustomerCache)
	customerSvcV2 := service.NewCustomerService(mongoCustomerRps, redisStreamCustomerCache)

	// HTTP Handlers
	authHTTPHandler := handlers.NewAuthHTTPHandler(authSvc)
	customerHTTPHandlerV1 := handlers.NewCustomerHTTPHandler(customerSvcV1)
	customerHTTPHandlerV2 := handlers.NewCustomerHTTPHandler(customerSvcV2)
	imageHandler := handlers.NewImageHTTPHandler()

	// gRPC Handlers
	authGrpcHandler := handlers.NewAuthGrpcHandler(authSvc)
	customerGrpcHandler := handlers.NewCustomerGrpcHandler(customerSvcV1)

	// interceptors
	authInterceptor := interceptors.AuthUnaryInterceptor(jwtValidator, interceptors.UnaryApplicableForService("CustomerService"))
	validatorInterceptor := interceptors.ValidatorUnaryInterceptor(true)
	errorInterceptor := interceptors.ErrorUnaryInterceptor()

	images := e.Group("/images")
	images.POST("/upload", imageHandler.Upload)
	images.GET("/:name/download", imageHandler.Download)
	images.Use(echoMw.StaticWithConfig(echoMw.StaticConfig{
		Root:   "images",
		Browse: true,
	}))

	// API routes
	api := e.Group("/api")

	// auth
	apiAuth := api.Group("/auth")
	apiAuth.POST("/signup", authHTTPHandler.Signup)
	apiAuth.POST("/login", authHTTPHandler.Login)
	apiAuth.POST("/logout", authHTTPHandler.Logout)
	apiAuth.POST("/refresh", authHTTPHandler.Refresh)

	// customers v1
	apiCustomersV1 := api.Group("/v1/customers", authorizeMw)
	apiCustomersV1.GET("", customerHTTPHandlerV1.GetAll)
	apiCustomersV1.GET("/:id", customerHTTPHandlerV1.Get)
	apiCustomersV1.POST("", customerHTTPHandlerV1.Post)
	apiCustomersV1.PUT("/:id", customerHTTPHandlerV1.Put)
	apiCustomersV1.DELETE("/:id", customerHTTPHandlerV1.DeleteByID)

	// customers v2
	apiCustomersV2 := api.Group("/v2/customers", authorizeMw)
	apiCustomersV2.GET("", customerHTTPHandlerV2.GetAll)
	apiCustomersV2.GET("/:id", customerHTTPHandlerV2.Get)
	apiCustomersV2.POST("", customerHTTPHandlerV2.Post)
	apiCustomersV2.PUT("/:id", customerHTTPHandlerV2.Put)
	apiCustomersV2.DELETE("/:id", customerHTTPHandlerV2.DeleteByID)

	e.GET("/swagger/*", echoSwagger.WrapHandler)

	shutdownCh := make(chan os.Signal, 1)
	errorCh := make(chan error, 1)
	signal.Notify(shutdownCh, os.Interrupt)

	// start HTTP server
	go func() {
		logrus.Infof("Starting HTTP server at port :%d", httpPort)
		if startErr := e.Start(fmt.Sprintf(":%d", httpPort)); startErr != nil {
			logrus.Error("HTTP server raised error")
			errorCh <- startErr
		}
	}()

	// start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		logrus.Fatal(err)
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
		logrus.Infof("Starting gRPC server at port :%d", grpcPort)
		if serveErr := grpcSvc.Serve(lis); serveErr != nil {
			logrus.Error("gRPC server raised error")
			errorCh <- serveErr
		}
	}()

	// start redis steam listen loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go readCustomersStream(ctx, redisClient, inMemoryCustomerCache)

	select {
	case <-shutdownCh:
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		logrus.Info("shutdown signal has been sent")
		logrus.Info("stopping the HTTP server...")
		if err := e.Shutdown(ctx); err != nil {
			logrus.Errorf("failed to stop server gracefully - %v", err)
		}

		logrus.Info("stopping the gRPC server...")
		grpcSvc.Stop()
	case err := <-errorCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logrus.Errorf("shutting down the servers because of unexpected error - %v", err)
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
		DB:         cfg.DB,
		MaxRetries: cfg.MaxRetries,
		PoolSize:   cfg.PoolSize,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("didn't get response from redis after sending ping request - %w", err)
	}
	return client, nil
}

func setupLogger() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetReportCaller(true)
}

func echoValidator() (echo.Validator, error) {
	v := validator.New()

	// store json tag fields, so can be handled on UI properly in struct PayloadErr -> field Field
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			return field.Name
		}
		return jsonName
	})

	enLocale := en.New()
	unvTranslator := ut.New(enLocale, enLocale)
	trans, ok := unvTranslator.GetTranslator("en")
	if !ok {
		return nil, errors.New("failed to find translator for en locale")
	}

	// register default translations
	if err := enTrans.RegisterDefaultTranslations(v, trans); err != nil {
		return nil, fmt.Errorf("failed to register en translations - %w", err)
	}

	return validation.Echo(v, trans), nil
}

func readCustomersStream(ctx context.Context, client *redis.Client, customerCache cache.CustomerCacheRepository) {
	key := "$"
	logrus.Info("starting to read customers redis stream")

XRead:
	for {
		select {
		case <-ctx.Done():
			break XRead
		default:
			logrus.Infof("waiting for new messages starting from %s", key)
			streams, err := client.XRead(ctx, &redis.XReadArgs{
				Streams: []string{"customers-stream", key},
				Count:   readStreamMessagesMaxCount,
				Block:   readStreamBlockTime,
			}).Result()
			if err != nil {
				logrus.Errorf("error occurred on reading message from stream - %v", err)
				continue
			}

			logrus.Info("messages were received")

			for _, stream := range streams {
				for _, m := range stream.Messages {
					logrus.Info("number of message received = ", len(stream.Messages))

					key = m.ID
					if err := processStreamMessage(ctx, customerCache, m); err != nil {
						logrus.Errorf("error occurred on message %s processing - %v", key, err)
					}
				}
			}
		}
	}
}

func processStreamMessage(ctx context.Context, customerCache cache.CustomerCacheRepository, m redis.XMessage) error {
	op, ok := m.Values["op"].(string)
	if !ok || op == "" {
		return errors.New("message has incorrect format - op field is missing, skipped")
	}

	value, ok := m.Values["value"].(string)
	if !ok {
		return errors.New("message has incorrect format - value field is missing, skipped")
	}

	logrus.Infof("%s operation is requested", op)

	writeCtx, cancel := context.WithTimeout(ctx, cacheWriteTimeout)
	defer cancel()

	switch op {
	case "create":
		var c model.Customer
		if err := msgpack.Unmarshal([]byte(value), &c); err != nil {
			return fmt.Errorf("failed to deserialize customer - %w", err)
		}

		if err := customerCache.Create(writeCtx, &c); err != nil {
			return fmt.Errorf("failed to create customer entry in cache - %w", err)
		}
	case "delete":
		if err := customerCache.DeleteByID(writeCtx, value); err != nil {
			return fmt.Errorf("failed to delete customer entry from cache - %w", err)
		}
	}

	return nil
}
