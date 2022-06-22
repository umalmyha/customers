package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/umalmyha/customers/internal/handlers"
	"github.com/umalmyha/customers/internal/repository"
	"github.com/umalmyha/customers/internal/service"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const DefaultPort = 3000
const DefaultShutdownTimeout = 10 * time.Second
const ServerStartupTimeout = 10 * time.Second

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), ServerStartupTimeout)
	defer cancel()

	pgPool, err := postgresql(ctx)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer pgPool.Close()

	mongoClient, err := mongodb(ctx)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer func() {
		if err = mongoClient.Disconnect(ctx); err != nil {
			log.Fatalf(err.Error())
		}
	}()

	start(pgPool, mongoClient)
}

func postgresql(ctx context.Context) (*pgxpool.Pool, error) {
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	database := os.Getenv("POSTGRES_DB")
	sslMode := os.Getenv("POSTGRES_SLL_MODE")
	poolMaxConn := os.Getenv("POSTGRES_POOL_MAX_CONN")
	dsn := fmt.Sprintf("user=%s password=%s host=pg-customers port=5432 dbname=%s sslmode=%s pool_max_conns=%s", user, password, database, sslMode, poolMaxConn)

	pool, err := pgxpool.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to establish connection to db - %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("didn't get response from database after sending ping request - %w", err)
	}
	return pool, nil
}

func mongodb(ctx context.Context) (*mongo.Client, error) {
	user := os.Getenv("MONGO_USER")
	password := os.Getenv("MONGO_PASSWORD")
	maxPoolSize := os.Getenv("MONGO_MAX_POOL_SIZE")
	uri := fmt.Sprintf("mongodb://%s:%s@mongo-customers:27017/?maxPoolSize=%s", user, password, maxPoolSize)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}
	return client, nil
}

func start(pgPool *pgxpool.Pool, mongoClient *mongo.Client) {
	app := app(pgPool, mongoClient)

	shutdownCh := make(chan os.Signal, 1)
	errorCh := make(chan error, 1)
	signal.Notify(shutdownCh, os.Interrupt)

	go func() {
		errorCh <- app.Start(fmt.Sprintf(":%d", DefaultPort))
	}()

	select {
	case <-shutdownCh:
		ctx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
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

func app(pgPool *pgxpool.Pool, mongoClient *mongo.Client) *echo.Echo {
	e := echo.New()

	handlerV1 := handlerV1(pgPool)
	handlerV2 := handlerV2(mongoClient)

	apiV1 := e.Group("/api/v1")
	custV1 := apiV1.Group("/customers")
	custV1.GET("", handlerV1.GetAll)
	custV1.GET("/:id", handlerV1.Get)
	custV1.POST("", handlerV1.Post)
	custV1.PUT("/:id", handlerV1.Put)
	custV1.DELETE("/:id", handlerV1.DeleteById)

	apiV2 := e.Group("/api/v2")
	custV2 := apiV2.Group("/customers")
	custV2.GET("", handlerV2.GetAll)
	custV2.GET("/:id", handlerV2.Get)
	custV2.POST("", handlerV2.Post)
	custV2.PUT("/:id", handlerV2.Put)
	custV2.DELETE("/:id", handlerV2.DeleteById)

	return e
}

func handlerV1(pgPool *pgxpool.Pool) *handlers.CustomerHandler {
	custRepo := repository.NewPostgresCustomerRepository(pgPool)
	custSrv := service.NewCustomerService(custRepo)
	return handlers.NewCustomerHandler(custSrv)
}

func handlerV2(mongoClient *mongo.Client) *handlers.CustomerHandler {
	custRepo := repository.NewMongoCustomerRepository(mongoClient)
	custSrv := service.NewCustomerService(custRepo)
	return handlers.NewCustomerHandler(custSrv)
}
