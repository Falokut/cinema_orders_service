package mongorepository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const timeoutPingDuration = 10 * time.Second

func NewMongoDB(connStr string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutPingDuration)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connStr))
	if err != nil {
		return nil, err
	}
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), timeoutPingDuration)
	defer cancel()
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}
