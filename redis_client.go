package milvus_cdc

import (
	"context"

	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
	redis *redis.Client
}

func NewRedisClient(redis *redis.Client) *RedisClient {
	return &RedisClient{
		redis: redis,
	}
}

func (r *RedisClient) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return r.redis.Subscribe(ctx, channel)
}

func (r *RedisClient) Publish(ctx context.Context, channel, message string) (int64, error) {
	return r.redis.Publish(ctx, channel, message).Result()
}
