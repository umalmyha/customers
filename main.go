package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/umalmyha/customers/internal/infra"
	"go.mongodb.org/mongo-driver/mongo"
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

	pgPool, err := infra.Postgresql(ctx)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer pgPool.Close()

	mongoClient, err := infra.Mongodb(ctx)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer func() {
		if err = mongoClient.Disconnect(ctx); err != nil {
			log.Fatalf(err.Error())
		}
	}()

	authCfg, err := infra.BuildAuthConfig()
	if err != nil {
		log.Fatalf(err.Error())
	}

	start(pgPool, mongoClient, authCfg)
}

func start(pgPool *pgxpool.Pool, mongoClient *mongo.Client, authCfg infra.AuthConfig) {
	router := infra.Router(pgPool, mongoClient, authCfg)

	shutdownCh := make(chan os.Signal, 1)
	errorCh := make(chan error, 1)
	signal.Notify(shutdownCh, os.Interrupt)

	go func() {
		errorCh <- router.Start(fmt.Sprintf(":%d", DefaultPort))
	}()

	select {
	case <-shutdownCh:
		ctx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
		defer cancel()

		router.Logger.Infof("shutdown signal has been sent, stopping the server...")
		if err := router.Shutdown(ctx); err != nil {
			router.Logger.Fatalf("failed to stop server gracefully - %s", err)
		}
	case err := <-errorCh:
		if !errors.Is(err, http.ErrServerClosed) {
			router.Logger.Fatalf("shutting down the server, unexpected error occurred - %s", err)
		}
	}
}
