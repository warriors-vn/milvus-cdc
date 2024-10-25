package milvus_cdc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/milvus-io/milvus-sdk-go/milvus"
)

type RedisBroker struct {
	sig      chan os.Signal
	redisCli *RedisClient
	milvus   []milvus.MilvusClient
}

func NewRedisBroker(redis *redis.Client, milvus []milvus.MilvusClient) *RedisBroker {
	redisCli := NewRedisClient(redis)
	return &RedisBroker{
		sig:      make(chan os.Signal, 1),
		redisCli: redisCli,
		milvus:   milvus,
	}
}

func (rb *RedisBroker) Start(channel string) error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	subscriber := rb.redisCli.Subscribe(ctx, channel)
	for i := 0; i < len(rb.milvus); i++ {
		go func(idx int) {
			for {
				message, err := subscriber.ReceiveMessage(ctx)
				if err != nil {
					return
				}

				_ = rb.handle(message.Payload, idx)
			}
		}(i)
	}

	<-rb.sig
	cancelFunc()

	return nil
}

func (rb *RedisBroker) handle(msg string, idx int) error {
	var message MessageCDC

	err := json.Unmarshal([]byte(msg), &message)
	if err != nil {
		return err
	}

	return rb.sync(&message, idx)
}

func (rb *RedisBroker) sync(message *MessageCDC, idx int) error {
	if message == nil {
		return fmt.Errorf("message cdc not found")
	}

	switch message.action {
	case Insert:
		return rb.insert(message, idx)
	case Update:
		return nil
	case Delete:
		return nil
	}

	return fmt.Errorf("the action is invalid")
}

func (rb *RedisBroker) insert(message *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vByte, err := hex.DecodeString(message.vector)
	if err != nil {
		return err
	}

	_, status, err := rb.milvus[idx].Insert(ctx, &milvus.InsertParam{
		CollectionName: "collectionName",
		PartitionTag:   "partitionTag",
		RecordArray: []milvus.Entity{
			{
				FloatData: DecodeUnsafeF32(vByte),
			},
		},
		IDArray: []int64{message.id},
	})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return errors.New(status.GetMessage())
	}

	return nil
}
