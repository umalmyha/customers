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
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const DefaultPort = 3000
const DefaultShutdownTimeout = 10 * time.Second
const DefaultDatabaseConnectTimeout = 5 * time.Second

func main() {
	pool, err := connectToDb()
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer pool.Close()

	start(pool)
}

func connectToDb() (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDatabaseConnectTimeout)
	defer cancel()

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

func start(pool *pgxpool.Pool) {
	app := app(pool)

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

func app(pool *pgxpool.Pool) *echo.Echo {
	custRepo := repository.NewPostgresCustomerRepository(pool)
	custSrv := service.NewCustomerService(custRepo)
	custHandler := handlers.NewCustomerHandler(custSrv)

	e := echo.New()

	apiGrp := e.Group("/api")

	custGrp := apiGrp.Group("/customers")
	custGrp.GET("", custHandler.GetAll)
	custGrp.GET("/:id", custHandler.Get)
	custGrp.POST("", custHandler.Post)
	custGrp.PUT("/:id", custHandler.Put)
	custGrp.DELETE("/:id", custHandler.DeleteById)

	return e
}
