package milvus_cdc

import (
	"context"

	"github.com/go-redis/redis/v8"
)

type IRedisClientInterface interface {
	Subscribe(ctx context.Context, channel string) *redis.PubSub
	Publish(ctx context.Context, channel, message string) (int64, error)
}
