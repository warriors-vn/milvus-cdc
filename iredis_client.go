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

	// XGroupCreateMkStream creates stream (if it doesn't exist yet) and a consumer
	// group on it starting at start (e.g. "0" for the whole history, "$" for only
	// new entries). It is not an error for the group to already exist.
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) error
	// XReadGroup reads new entries (id ">") for consumer within group, blocking up
	// to block (0 blocks forever, matching BRPop's timeout semantics).
	XReadGroup(ctx context.Context, group, consumer, stream string, block time.Duration) ([]redis.XMessage, error)
	XAck(ctx context.Context, stream, group string, ids ...string) error
	XAdd(ctx context.Context, stream string, values map[string]interface{}) (string, error)
}
