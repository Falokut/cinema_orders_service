package rediscache

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func NewRedisCache(opt *redis.Options) (*redis.Client, error) {
	rdb := redis.NewClient(opt)
	if rdb == nil {
		return nil, errors.New("can't create new redis client")
	}

	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		return nil, fmt.Errorf("connection is not established: %s", err.Error())
	}

	return rdb, nil
}
