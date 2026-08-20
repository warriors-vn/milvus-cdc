package milvus_cdc

import (
	"context"

	kafka "github.com/segmentio/kafka-go"
)

// IKafkaClientInterface is the seam KafkaBroker consumes through, so tests can inject a
// fake instead of a live Kafka connection.
type IKafkaClientInterface interface {
	// FetchMessage reads the next message without committing its offset (see
	// CommitMessages). It blocks until a message is available or ctx is done.
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Publish(ctx context.Context, key, value []byte) error
}

// KafkaClient wraps a *kafka.Reader (for consuming, via KafkaBroker) and an optional
// *kafka.Writer (for producing). reader must be configured with a GroupID: KafkaBroker
// relies on consumer-group offset commits for its at-least-once delivery guarantee.
type KafkaClient struct {
	reader *kafka.Reader
	writer *kafka.Writer
}

func NewKafkaClient(reader *kafka.Reader, writer *kafka.Writer) *KafkaClient {
	return &KafkaClient{reader: reader, writer: writer}
}

func (c *KafkaClient) FetchMessage(ctx context.Context) (kafka.Message, error) {
	return c.reader.FetchMessage(ctx)
}

func (c *KafkaClient) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	return c.reader.CommitMessages(ctx, msgs...)
}

func (c *KafkaClient) Publish(ctx context.Context, key, value []byte) error {
	return c.writer.WriteMessages(ctx, kafka.Message{Key: key, Value: value})
}
