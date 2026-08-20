package milvus_cdc

import (
	"fmt"
	"sync"
	"time"
)

// OnRabbitMQConnErrorFunc is invoked when consuming from RabbitMQ fails (e.g.
// connection dropped) or when acknowledging a delivery fails.
type OnRabbitMQConnErrorFunc func(err error)

type RabbitMQBrokerOption func(*RabbitMQBroker)

func WithRabbitMQOnMessage(fn OnMessageFunc) RabbitMQBrokerOption {
	return func(rb *RabbitMQBroker) {
		rb.dispatcher.onMessage = fn
	}
}

func WithRabbitMQOnConnError(fn OnRabbitMQConnErrorFunc) RabbitMQBrokerOption {
	return func(rb *RabbitMQBroker) {
		rb.onConnError = fn
	}
}

// WithRabbitMQMaxRetries overrides the default MaxHandleRetries for this broker.
func WithRabbitMQMaxRetries(n int) RabbitMQBrokerOption {
	return func(rb *RabbitMQBroker) {
		rb.dispatcher.maxRetries = n
	}
}

// WithRabbitMQRetryDelay overrides the default HandleRetryDelay for this broker.
func WithRabbitMQRetryDelay(d time.Duration) RabbitMQBrokerOption {
	return func(rb *RabbitMQBroker) {
		rb.dispatcher.retryDelay = d
	}
}

// RabbitMQBroker delivers CDC messages consumed from a RabbitMQ queue to a set of
// milvus clients. Unlike RedisBroker's Queue pattern, RabbitMQ gives it real
// at-least-once delivery: a delivery is only acknowledged once every milvus client has
// applied it; if any of them still fails after retries, the delivery is nacked and
// requeued so it will be redelivered.
type RabbitMQBroker struct {
	stopOnce    sync.Once
	done        chan struct{}
	rabbitCli   IRabbitMQClientInterface
	dispatcher  dispatcher
	onConnError OnRabbitMQConnErrorFunc
}

func NewRabbitMQBroker(rabbitCli IRabbitMQClientInterface, milvus []IMilvusClientInterface, opts ...RabbitMQBrokerOption) *RabbitMQBroker {
	rb := &RabbitMQBroker{
		done:       make(chan struct{}),
		rabbitCli:  rabbitCli,
		dispatcher: newDispatcher(milvus),
	}

	for _, opt := range opts {
		opt(rb)
	}

	return rb
}

func (rb *RabbitMQBroker) reportConnError(err error) {
	if rb.onConnError != nil {
		rb.onConnError(err)
	}
}

// Start consumes from the given queue until the delivery channel is closed or Stop is
// called. pattern is accepted to satisfy IBrokerFactory; only Queue is supported today.
func (rb *RabbitMQBroker) Start(queue, pattern string) error {
	if pattern != Queue {
		return fmt.Errorf("pattern is invalid")
	}

	deliveries, err := rb.rabbitCli.Consume(queue)
	if err != nil {
		return err
	}

	for {
		select {
		case <-rb.done:
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return nil
			}

			if errBroadcast := rb.dispatcher.broadcast(string(delivery.Body)); errBroadcast != nil {
				if errNack := delivery.Nack(false, true); errNack != nil {
					rb.reportConnError(errNack)
				}
				continue
			}

			if errAck := delivery.Ack(false); errAck != nil {
				rb.reportConnError(errAck)
			}
		}
	}
}

func (rb *RabbitMQBroker) Stop() {
	rb.stopOnce.Do(func() {
		close(rb.done)
	})
}
