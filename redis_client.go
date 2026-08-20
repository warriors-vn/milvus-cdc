package milvus_cdc

import (
	"context"
	"strings"
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

func (r *RedisClient) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	err := r.redis.XGroupCreateMkStream(ctx, stream, group, start).Err()
	if err != nil && strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}

	return err
}

func (r *RedisClient) XReadGroup(ctx context.Context, group, consumer, stream string, block time.Duration) ([]redis.XMessage, error) {
	res, err := r.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Block:    block,
	}).Result()
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return nil, nil
	}

	return res[0].Messages, nil
}

func (r *RedisClient) XAck(ctx context.Context, stream, group string, ids ...string) error {
	return r.redis.XAck(ctx, stream, group, ids...).Err()
}

func (r *RedisClient) XAdd(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	return r.redis.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: values}).Result()
}
