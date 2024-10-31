package milvus_cdc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type RedisBroker struct {
	sig      chan os.Signal
	redisCli *RedisClient
	milvus   []*MilvusClient
}

func NewRedisBroker(redis *redis.Client, milvus []*MilvusClient) *RedisBroker {
	redisCli := NewRedisClient(redis)

	return &RedisBroker{
		sig:      make(chan os.Signal, 1),
		redisCli: redisCli,
		milvus:   milvus,
	}
}

func (rb *RedisBroker) Start(channel, pattern string) error {
	switch pattern {
	case PubSub:
		return rb.pubSub(channel)
	case Queue:
		return rb.queue(channel)
	}

	return fmt.Errorf("pattern is invalid")
}

func (rb *RedisBroker) Stop() {
	rb.sig <- syscall.SIGKILL
}

func (rb *RedisBroker) pubSub(channel string) error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	for i := 0; i < len(rb.milvus); i++ {
		go func(idx int) {
			subscriber := rb.redisCli.Subscribe(ctx, channel)
			for {
				message, err := subscriber.ReceiveMessage(ctx)
				if err != nil {
					return
				}

				errHandle := rb.handle(message.Payload, idx)
				if errHandle != nil {
					logrus.Errorf("handle message is failed with input %v and err %v", message, errHandle)
					continue
				}

				logrus.Infof("handle message is successfully with input %v", message)
			}
		}(i)
	}

	<-rb.sig
	cancelFunc()

	return nil
}

func (rb *RedisBroker) queue(channel string) error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		for {
			// using BRPop will wait with a timeout if the queue is empty. If timeout is 0 it will wait forever
			message, err := rb.redisCli.BRPop(ctx, channel, 0)
			if err != nil {
				return
			}

			if len(message) < 2 {
				continue
			}

			var wg sync.WaitGroup
			for i := 0; i < len(rb.milvus); i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					_ = rb.handle(message[1], idx)
				}(i)
			}

			wg.Wait()
		}
	}()

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

	if len(rb.milvus) <= idx {
		return fmt.Errorf("milvus client not found")
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
	return rb.milvus[idx].Insert(cdc.Vector, cdc.CollectionName, cdc.PartitionTag, cdc.Id)
}

func (rb *RedisBroker) delete(cdc *MessageCDC, idx int) error {
	return rb.milvus[idx].Delete(cdc.CollectionName, cdc.PartitionTag, cdc.Id)
}

func (rb *RedisBroker) dropCollection(cdc *MessageCDC, idx int) error {
	return rb.milvus[idx].DropCollection(cdc.CollectionName)
}

func (rb *RedisBroker) createCollection(cdc *MessageCDC, idx int) error {
	return rb.milvus[idx].CreateCollection(cdc.CollectionName, cdc.Dimension, cdc.IndexFileSize, cdc.MetricType)
}

func (rb *RedisBroker) createIndex(cdc *MessageCDC, idx int) error {
	return rb.milvus[idx].CreateIndex(cdc.CollectionName, cdc.NList, cdc.IndexType)
}

func (rb *RedisBroker) dropIndex(cdc *MessageCDC, idx int) error {
	return rb.milvus[idx].DropIndex(cdc.CollectionName)
}

func (rb *RedisBroker) createPartition(cdc *MessageCDC, idx int) error {
	return rb.milvus[idx].CreatePartition(cdc.CollectionName, cdc.PartitionTag)
}

func (rb *RedisBroker) dropPartition(cdc *MessageCDC, idx int) error {
	return rb.milvus[idx].DropPartition(cdc.CollectionName, cdc.PartitionTag)
}
