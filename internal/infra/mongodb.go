package infra

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"os"
)

func Mongodb(ctx context.Context) (*mongo.Client, error) {
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
