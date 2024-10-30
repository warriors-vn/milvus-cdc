package milvus_cdc

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type IRedisClientInterface interface {
	Subscribe(ctx context.Context, channel string) *redis.PubSub
	Publish(ctx context.Context, channel, message string) (int64, error)
	LPush(ctx context.Context, queue string, value interface{}) (int64, error)
	BRPop(ctx context.Context, queue string, timeout time.Duration) ([]string, error)
}
