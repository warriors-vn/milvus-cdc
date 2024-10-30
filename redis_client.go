package milvus_cdc

import (
	"context"
	"time"

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

func (r *RedisClient) LPush(ctx context.Context, queue string, value interface{}) (int64, error) {
	return r.redis.LPush(ctx, queue, value).Result()
}

func (r *RedisClient) BRPop(ctx context.Context, queue string, timeout time.Duration) ([]string, error) {
	return r.redis.BRPop(ctx, timeout, queue).Result()
}
