package milvus_cdc

import (
	"context"
	"encoding/hex"
	"encoding/json"
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
	timeout  time.Duration
}

func NewRedisBroker(redis *redis.Client, milvus []milvus.MilvusClient, timeout time.Duration) *RedisBroker {
	redisCli := NewRedisClient(redis)
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &RedisBroker{
		sig:      make(chan os.Signal, 1),
		redisCli: redisCli,
		milvus:   milvus,
		timeout:  timeout,
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

	switch message.Action {
	case Insert:
		return rb.insert(message, idx)
	case Delete:
		return rb.delete(message, idx)
	case CreateCollection:
		return rb.createCollection(message, idx)
	case DropCollection:
		return rb.dropCollection(message, idx)
	case CreatePartition:
		return rb.createPartition(message, idx)
	case DropPartition:
		return rb.dropPartition(message, idx)
	case CreateIndex:
		return rb.createIndex(message, idx)
	case DropIndex:
		return rb.dropIndex(message, idx)
	}

	return fmt.Errorf("the action is invalid")
}

func (rb *RedisBroker) insert(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	vByte, err := hex.DecodeString(cdc.Vector)
	if err != nil {
		return err
	}

	_, status, err := rb.milvus[idx].Insert(ctx, &milvus.InsertParam{
		CollectionName: cdc.CollectionName,
		PartitionTag:   cdc.PartitionTag,
		RecordArray: []milvus.Entity{
			{
				FloatData: DecodeUnsafeF32(vByte),
			},
		},
		IDArray: []int64{cdc.Id},
	})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (rb *RedisBroker) delete(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	status, err := rb.milvus[idx].DeleteEntityByID(ctx, cdc.CollectionName,
		cdc.PartitionTag, []int64{cdc.Id})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (rb *RedisBroker) dropCollection(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	status, err := rb.milvus[idx].DropCollection(ctx, cdc.CollectionName)
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (rb *RedisBroker) createCollection(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	status, err := rb.milvus[idx].CreateCollection(ctx, milvus.CollectionParam{
		CollectionName: cdc.CollectionName,
		Dimension:      cdc.Dimension,
		IndexFileSize:  cdc.IndexFileSize,
		MetricType:     cdc.MetricType,
	})

	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (rb *RedisBroker) createIndex(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	indexParam := &milvus.IndexParam{
		CollectionName: cdc.CollectionName,
		IndexType:      milvus.IndexType(cdc.IndexType),
		ExtraParams:    cdc.ExtraParams,
	}

	status, err := rb.milvus[idx].CreateIndex(ctx, indexParam)
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (rb *RedisBroker) dropIndex(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	status, err := rb.milvus[idx].DropIndex(ctx, cdc.CollectionName)
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (rb *RedisBroker) createPartition(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	status, err := rb.milvus[idx].CreatePartition(ctx, milvus.PartitionParam{
		CollectionName: cdc.CollectionName,
		PartitionTag:   cdc.PartitionTag,
	})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (rb *RedisBroker) dropPartition(cdc *MessageCDC, idx int) error {
	ctx, cancel := context.WithTimeout(context.Background(), rb.timeout)
	defer cancel()

	status, err := rb.milvus[idx].DropPartition(ctx, milvus.PartitionParam{
		CollectionName: cdc.CollectionName,
		PartitionTag:   cdc.PartitionTag,
	})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}
