package milvus_cdc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// OnMessageFunc is invoked after each message is processed by a milvus client, idx
// being its index in the milvus client slice passed to NewRedisBroker. err is nil on
// success.
type OnMessageFunc func(msg string, idx int, err error)

// OnConnErrorFunc is invoked when the underlying redis subscription/queue read fails
// (e.g. connection dropped). It is not called on a graceful Stop().
type OnConnErrorFunc func(err error)

type RedisBrokerOption func(*RedisBroker)

func WithOnMessage(fn OnMessageFunc) RedisBrokerOption {
	return func(rb *RedisBroker) {
		rb.dispatcher.onMessage = fn
	}
}

func WithOnConnError(fn OnConnErrorFunc) RedisBrokerOption {
	return func(rb *RedisBroker) {
		rb.onConnError = fn
	}
}

// WithMaxRetries overrides the default MaxHandleRetries for this broker.
func WithMaxRetries(n int) RedisBrokerOption {
	return func(rb *RedisBroker) {
		rb.dispatcher.maxRetries = n
	}
}

// WithRetryDelay overrides the default HandleRetryDelay for this broker.
func WithRetryDelay(d time.Duration) RedisBrokerOption {
	return func(rb *RedisBroker) {
		rb.dispatcher.retryDelay = d
	}
}

// WithStreamGroup sets the consumer group used by the Stream pattern (default
// DefaultStreamGroup). Only meaningful when Start is called with Stream.
func WithStreamGroup(group string) RedisBrokerOption {
	return func(rb *RedisBroker) {
		rb.streamGroup = group
	}
}

// WithStreamConsumer sets this broker's consumer name within the stream group
// (default: a value unique to this process). Give every broker instance sharing a
// group a distinct consumer name, otherwise Redis Streams' fan-out across consumers
// in the group won't work as expected.
func WithStreamConsumer(consumer string) RedisBrokerOption {
	return func(rb *RedisBroker) {
		rb.streamConsumer = consumer
	}
}

type RedisBroker struct {
	stopOnce       sync.Once
	done           chan struct{}
	redisCli       IRedisClientInterface
	dispatcher     dispatcher
	onConnError    OnConnErrorFunc
	streamGroup    string
	streamConsumer string
}

func NewRedisBroker(redisCli IRedisClientInterface, milvus []IMilvusClientInterface, opts ...RedisBrokerOption) *RedisBroker {
	rb := &RedisBroker{
		done:           make(chan struct{}),
		redisCli:       redisCli,
		dispatcher:     newDispatcher(milvus),
		streamGroup:    DefaultStreamGroup,
		streamConsumer: fmt.Sprintf("consumer-%d", time.Now().UnixNano()),
	}

	for _, opt := range opts {
		opt(rb)
	}

	return rb
}

func (rb *RedisBroker) reportConnError(err error) {
	if rb.onConnError != nil && !errors.Is(err, context.Canceled) {
		rb.onConnError(err)
	}
}

func (rb *RedisBroker) Start(channel, pattern string) error {
	switch pattern {
	case PubSub:
		return rb.pubSub(channel)
	case Queue:
		return rb.queue(channel)
	case Stream:
		return rb.stream(channel)
	}

	return fmt.Errorf("pattern is invalid")
}

func (rb *RedisBroker) Stop() {
	rb.stopOnce.Do(func() {
		close(rb.done)
	})
}

func (rb *RedisBroker) pubSub(channel string) error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	for i := 0; i < len(rb.dispatcher.milvus); i++ {
		go func(idx int) {
			subscriber := rb.redisCli.Subscribe(ctx, channel)
			for {
				message, err := subscriber.ReceiveMessage(ctx)
				if err != nil {
					rb.reportConnError(err)
					return
				}

				errHandle := rb.dispatcher.handle(message.Payload, idx)
				rb.dispatcher.reportMessage(message.Payload, idx, errHandle)
			}
		}(i)
	}

	<-rb.done
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
				rb.reportConnError(err)
				return
			}

			if len(message) < 2 {
				continue
			}

			rb.dispatcher.broadcast(message[1])
		}
	}()

	<-rb.done
	cancelFunc()

	return nil
}

// stream implements the Stream pattern: unlike Queue (BRPop, which removes a message
// from the list the instant it's received, ack or not), it uses a Redis Streams
// consumer group so a message is only acknowledged (XACK) once every milvus client has
// applied it. If any of them still fails after retries, the message is left
// unacknowledged as a pending entry, ready to be reclaimed and retried later (e.g. via
// XAUTOCLAIM/XCLAIM in a periodic sweep, or by a restarted consumer with the same
// group/consumer name), giving it the same at-least-once guarantee as RabbitMQBroker
// and KafkaBroker.
func (rb *RedisBroker) stream(streamName string) error {
	ctx, cancelFunc := context.WithCancel(context.Background())

	if err := rb.redisCli.XGroupCreateMkStream(ctx, streamName, rb.streamGroup, "0"); err != nil {
		cancelFunc()
		return err
	}

	go func() {
		for {
			messages, err := rb.redisCli.XReadGroup(ctx, rb.streamGroup, rb.streamConsumer, streamName, 0)
			if err != nil {
				rb.reportConnError(err)
				return
			}

			for _, msg := range messages {
				payload, _ := msg.Values[StreamPayloadField].(string)

				if errBroadcast := rb.dispatcher.broadcast(payload); errBroadcast != nil {
					continue
				}

				if errAck := rb.redisCli.XAck(ctx, streamName, rb.streamGroup, msg.ID); errAck != nil {
					rb.reportConnError(errAck)
				}
			}
		}
	}()

	<-rb.done
	cancelFunc()

	return nil
}
