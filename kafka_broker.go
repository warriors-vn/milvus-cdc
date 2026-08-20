package milvus_cdc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// OnKafkaConnErrorFunc is invoked when fetching from or committing to Kafka fails
// (e.g. connection dropped). It is not called on a graceful Stop().
type OnKafkaConnErrorFunc func(err error)

type KafkaBrokerOption func(*KafkaBroker)

func WithKafkaOnMessage(fn OnMessageFunc) KafkaBrokerOption {
	return func(kb *KafkaBroker) {
		kb.dispatcher.onMessage = fn
	}
}

func WithKafkaOnConnError(fn OnKafkaConnErrorFunc) KafkaBrokerOption {
	return func(kb *KafkaBroker) {
		kb.onConnError = fn
	}
}

// WithKafkaMaxRetries overrides the default MaxHandleRetries for this broker.
func WithKafkaMaxRetries(n int) KafkaBrokerOption {
	return func(kb *KafkaBroker) {
		kb.dispatcher.maxRetries = n
	}
}

// WithKafkaRetryDelay overrides the default HandleRetryDelay for this broker.
func WithKafkaRetryDelay(d time.Duration) KafkaBrokerOption {
	return func(kb *KafkaBroker) {
		kb.dispatcher.retryDelay = d
	}
}

// KafkaBroker delivers CDC messages fetched from a Kafka topic to a set of milvus
// clients. It relies on consumer-group offset commits for at-least-once delivery: a
// message's offset is only committed once every milvus client has applied it (after
// retries); if any of them still fails, the offset is left uncommitted, so the message
// is refetched (by this or another consumer in the group) after a restart/rebalance.
type KafkaBroker struct {
	stopOnce    sync.Once
	done        chan struct{}
	kafkaCli    IKafkaClientInterface
	dispatcher  dispatcher
	onConnError OnKafkaConnErrorFunc
}

func NewKafkaBroker(kafkaCli IKafkaClientInterface, milvus []IMilvusClientInterface, opts ...KafkaBrokerOption) *KafkaBroker {
	kb := &KafkaBroker{
		done:       make(chan struct{}),
		kafkaCli:   kafkaCli,
		dispatcher: newDispatcher(milvus),
	}

	for _, opt := range opts {
		opt(kb)
	}

	return kb
}

func (kb *KafkaBroker) reportConnError(err error) {
	if kb.onConnError != nil && !errors.Is(err, context.Canceled) {
		kb.onConnError(err)
	}
}

// Start fetches from the topic/group configured on the underlying reader until Stop is
// called. topic is accepted to satisfy IBrokerFactory but unused: it's fixed on the
// *kafka.Reader passed to NewKafkaClient. Only Queue is supported today.
func (kb *KafkaBroker) Start(topic, pattern string) error {
	if pattern != Queue {
		return fmt.Errorf("pattern is invalid")
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		for {
			msg, err := kb.kafkaCli.FetchMessage(ctx)
			if err != nil {
				kb.reportConnError(err)
				return
			}

			if errBroadcast := kb.dispatcher.broadcast(string(msg.Value)); errBroadcast != nil {
				continue
			}

			if errCommit := kb.kafkaCli.CommitMessages(ctx, msg); errCommit != nil {
				kb.reportConnError(errCommit)
			}
		}
	}()

	<-kb.done
	cancelFunc()

	return nil
}

func (kb *KafkaBroker) Stop() {
	kb.stopOnce.Do(func() {
		close(kb.done)
	})
}
