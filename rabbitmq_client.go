package milvus_cdc

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

// IRabbitMQClientInterface is the seam RabbitMQBroker consumes through, so tests can
// inject a fake instead of a live RabbitMQ connection.
type IRabbitMQClientInterface interface {
	// Consume starts delivering messages from queue. Every delivery must be
	// acknowledged by the caller (see amqp.Delivery.Ack / Nack).
	Consume(queue string) (<-chan amqp.Delivery, error)
	Publish(ctx context.Context, exchange, routingKey string, body []byte) error
}

type RabbitMQClient struct {
	ch *amqp.Channel
}

func NewRabbitMQClient(ch *amqp.Channel) *RabbitMQClient {
	return &RabbitMQClient{ch: ch}
}

func (r *RabbitMQClient) Consume(queue string) (<-chan amqp.Delivery, error) {
	return r.ch.Consume(queue, "", false, false, false, false, nil)
}

func (r *RabbitMQClient) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	return r.ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
