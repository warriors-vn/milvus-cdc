package milvus_cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type mockAcknowledger struct {
	mu       sync.Mutex
	acked    []uint64
	nacked   []uint64
	requeues []bool
}

func (m *mockAcknowledger) Ack(tag uint64, multiple bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acked = append(m.acked, tag)
	return nil
}

func (m *mockAcknowledger) Nack(tag uint64, multiple, requeue bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nacked = append(m.nacked, tag)
	m.requeues = append(m.requeues, requeue)
	return nil
}

func (m *mockAcknowledger) Reject(tag uint64, requeue bool) error {
	return nil
}

func (m *mockAcknowledger) snapshot() (acked, nacked []uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]uint64(nil), m.acked...), append([]uint64(nil), m.nacked...)
}

type mockRabbitMQClient struct {
	deliveries chan amqp.Delivery
	consumeErr error
}

func (m *mockRabbitMQClient) Consume(queue string) (<-chan amqp.Delivery, error) {
	if m.consumeErr != nil {
		return nil, m.consumeErr
	}
	return m.deliveries, nil
}

func (m *mockRabbitMQClient) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	return nil
}

func TestRabbitMQBroker_AcksOnSuccess(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	ack := &mockAcknowledger{}
	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{Body: raw, DeliveryTag: 1, Acknowledger: ack}

	milvusCli := &mockMilvusClient{}
	rb := NewRabbitMQBroker(&mockRabbitMQClient{deliveries: deliveries}, []IMilvusClientInterface{milvusCli})

	startDone := make(chan error, 1)
	go func() {
		startDone <- rb.Start("queue", Queue)
	}()

	deadline := time.After(2 * time.Second)
	for {
		acked, _ := ack.snapshot()
		if len(acked) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for delivery to be acked")
		case <-time.After(10 * time.Millisecond):
		}
	}

	rb.Stop()

	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("expected Start to return nil after Stop, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Start to return after Stop")
	}

	if milvusCli.calls() != 1 {
		t.Fatalf("expected 1 insert call, got %d", milvusCli.calls())
	}
}

func TestRabbitMQBroker_NacksAndRequeuesOnFailure(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	ack := &mockAcknowledger{}
	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{Body: raw, DeliveryTag: 1, Acknowledger: ack}

	permanentErr := errors.New("permanent")
	milvusCli := &mockMilvusClient{insertErrs: []error{permanentErr, permanentErr, permanentErr}}
	rb := NewRabbitMQBroker(&mockRabbitMQClient{deliveries: deliveries}, []IMilvusClientInterface{milvusCli}, WithRabbitMQRetryDelay(time.Millisecond))

	go func() {
		_ = rb.Start("queue", Queue)
	}()

	deadline := time.After(2 * time.Second)
	for {
		_, nacked := ack.snapshot()
		if len(nacked) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for delivery to be nacked")
		case <-time.After(10 * time.Millisecond):
		}
	}

	rb.Stop()

	ack.mu.Lock()
	defer ack.mu.Unlock()
	if len(ack.acked) != 0 {
		t.Fatalf("expected no acks, got %d", len(ack.acked))
	}
	if len(ack.requeues) != 1 || !ack.requeues[0] {
		t.Fatalf("expected the nack to request a requeue, got %v", ack.requeues)
	}
}

func TestRabbitMQBroker_Start_RejectsNonQueuePattern(t *testing.T) {
	rb := NewRabbitMQBroker(&mockRabbitMQClient{}, nil)

	err := rb.Start("queue", PubSub)
	if err == nil {
		t.Fatal("expected error for unsupported pattern")
	}
}

func TestRabbitMQBroker_Start_PropagatesConsumeError(t *testing.T) {
	consumeErr := errors.New("connection refused")
	rb := NewRabbitMQBroker(&mockRabbitMQClient{consumeErr: consumeErr}, nil)

	err := rb.Start("queue", Queue)
	if !errors.Is(err, consumeErr) {
		t.Fatalf("expected %v, got %v", consumeErr, err)
	}
}

func TestRabbitMQBroker_Stop_Idempotent(t *testing.T) {
	rb := NewRabbitMQBroker(&mockRabbitMQClient{}, nil)

	rb.Stop()
	rb.Stop()

	select {
	case <-rb.done:
	default:
		t.Fatal("expected done channel to be closed after Stop")
	}
}
