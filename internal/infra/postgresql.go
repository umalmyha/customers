package infra

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"os"
)

func Postgresql(ctx context.Context) (*pgxpool.Pool, error) {
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
