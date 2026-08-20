package milvus_cdc

import (
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MQTTBrokerOption func(*MQTTBroker)

func WithMQTTOnMessage(fn OnMessageFunc) MQTTBrokerOption {
	return func(mb *MQTTBroker) {
		mb.dispatcher.onMessage = fn
	}
}

// WithMQTTMaxRetries overrides the default MaxHandleRetries for this broker.
func WithMQTTMaxRetries(n int) MQTTBrokerOption {
	return func(mb *MQTTBroker) {
		mb.dispatcher.maxRetries = n
	}
}

// WithMQTTRetryDelay overrides the default HandleRetryDelay for this broker.
func WithMQTTRetryDelay(d time.Duration) MQTTBrokerOption {
	return func(mb *MQTTBroker) {
		mb.dispatcher.retryDelay = d
	}
}

// MQTTBroker delivers CDC messages received on an MQTT topic (e.g. from EMQX,
// Mosquitto, HiveMQ) to a set of milvus clients. A message is only acked once every
// milvus client has applied it (after retries); if any of them still fails, the
// message is left unacked. Unlike RabbitMQ, MQTT has no explicit nack/requeue: an
// unacked QoS>=1 message is only redelivered by the broker on the client's next
// reconnect with a persistent session, so this is a weaker at-least-once guarantee
// than RabbitMQBroker's.
type MQTTBroker struct {
	stopOnce   sync.Once
	done       chan struct{}
	mqttCli    IMQTTClientInterface
	dispatcher dispatcher
}

func NewMQTTBroker(mqttCli IMQTTClientInterface, milvus []IMilvusClientInterface, opts ...MQTTBrokerOption) *MQTTBroker {
	mb := &MQTTBroker{
		done:       make(chan struct{}),
		mqttCli:    mqttCli,
		dispatcher: newDispatcher(milvus),
	}

	for _, opt := range opts {
		opt(mb)
	}

	return mb
}

// Start subscribes to topic and processes messages until Stop is called. pattern is
// accepted to satisfy IBrokerFactory; only Queue is supported today.
func (mb *MQTTBroker) Start(topic, pattern string) error {
	if pattern != Queue {
		return fmt.Errorf("pattern is invalid")
	}

	err := mb.mqttCli.Subscribe(topic, func(_ mqtt.Client, msg mqtt.Message) {
		if errBroadcast := mb.dispatcher.broadcast(string(msg.Payload())); errBroadcast != nil {
			return
		}

		msg.Ack()
	})
	if err != nil {
		return err
	}

	<-mb.done

	return nil
}

func (mb *MQTTBroker) Stop() {
	mb.stopOnce.Do(func() {
		close(mb.done)
	})
}
