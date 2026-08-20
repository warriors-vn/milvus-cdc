package milvus_cdc

import (
	"context"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// IMQTTClientInterface is the seam MQTTBroker consumes through, so tests can inject a
// fake instead of a live MQTT broker connection (e.g. EMQX, Mosquitto, HiveMQ).
type IMQTTClientInterface interface {
	// Subscribe registers handler to be invoked for every message published to
	// topic. handler is called on a goroutine managed by the underlying MQTT
	// client (see paho's MessageHandler), not by the caller.
	Subscribe(topic string, handler mqtt.MessageHandler) error
	Publish(ctx context.Context, topic string, payload []byte) error
}

// MQTTClient wraps a paho mqtt.Client. Its at-least-once behaviour (see
// MQTTBroker) requires QoS >= 1 and the underlying client built with
// mqtt.NewClientOptions().SetAutoAckDisabled(true), otherwise the broker
// acks messages before they are known to have synced to every milvus client.
type MQTTClient struct {
	cli mqtt.Client
	qos byte
}

func NewMQTTClient(cli mqtt.Client, qos byte) *MQTTClient {
	return &MQTTClient{cli: cli, qos: qos}
}

func (c *MQTTClient) Subscribe(topic string, handler mqtt.MessageHandler) error {
	token := c.cli.Subscribe(topic, c.qos, func(client mqtt.Client, msg mqtt.Message) {
		handler(client, msg)
	})

	token.Wait()

	return token.Error()
}

func (c *MQTTClient) Publish(ctx context.Context, topic string, payload []byte) error {
	token := c.cli.Publish(topic, c.qos, false, payload)
	token.Wait()

	return token.Error()
}
