package milvus_cdc

import (
	"sync"
	"time"
)

type ChannelBrokerOption func(*ChannelBroker)

func WithChannelOnMessage(fn OnMessageFunc) ChannelBrokerOption {
	return func(cb *ChannelBroker) {
		cb.dispatcher.onMessage = fn
	}
}

// WithChannelMaxRetries overrides the default MaxHandleRetries for this broker.
func WithChannelMaxRetries(n int) ChannelBrokerOption {
	return func(cb *ChannelBroker) {
		cb.dispatcher.maxRetries = n
	}
}

// WithChannelRetryDelay overrides the default HandleRetryDelay for this broker.
func WithChannelRetryDelay(d time.Duration) ChannelBrokerOption {
	return func(cb *ChannelBroker) {
		cb.dispatcher.retryDelay = d
	}
}

// ChannelBroker delivers CDC messages read off an in-memory Go channel to a set of
// milvus clients. Unlike RedisBroker it needs no external broker process running, so
// it's useful for same-process producers, tests, or as a dependency-free default.
type ChannelBroker struct {
	stopOnce   sync.Once
	done       chan struct{}
	msgCh      <-chan string
	dispatcher dispatcher
}

// NewChannelBroker builds a broker that consumes JSON-encoded MessageCDC values off
// msgCh. The caller owns msgCh: it is responsible for producing onto it and for
// closing it once no more messages will be sent (Start returns once msgCh is closed
// and drained, or once Stop is called).
func NewChannelBroker(msgCh <-chan string, milvus []IMilvusClientInterface, opts ...ChannelBrokerOption) *ChannelBroker {
	cb := &ChannelBroker{
		done:       make(chan struct{}),
		msgCh:      msgCh,
		dispatcher: newDispatcher(milvus),
	}

	for _, opt := range opts {
		opt(cb)
	}

	return cb
}

// Start consumes msgCh until it is closed or Stop is called. channel and pattern are
// accepted to satisfy IBrokerFactory but are otherwise unused: the transport is fixed
// to msgCh at construction time. Each message is broadcast to every milvus client
// passed to NewChannelBroker, same as RedisBroker's Queue pattern.
func (cb *ChannelBroker) Start(channel, pattern string) error {
	for {
		select {
		case <-cb.done:
			return nil
		case msg, ok := <-cb.msgCh:
			if !ok {
				return nil
			}

			cb.dispatcher.broadcast(msg)
		}
	}
}

func (cb *ChannelBroker) Stop() {
	cb.stopOnce.Do(func() {
		close(cb.done)
	})
}
