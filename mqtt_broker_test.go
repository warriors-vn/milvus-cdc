package milvus_cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type mockMQTTMessage struct {
	payload []byte
	acked   int32
}

func (m *mockMQTTMessage) Duplicate() bool   { return false }
func (m *mockMQTTMessage) Qos() byte         { return 1 }
func (m *mockMQTTMessage) Retained() bool    { return false }
func (m *mockMQTTMessage) Topic() string     { return "topic" }
func (m *mockMQTTMessage) MessageID() uint16 { return 1 }
func (m *mockMQTTMessage) Payload() []byte   { return m.payload }
func (m *mockMQTTMessage) Ack()              { atomic.AddInt32(&m.acked, 1) }

func (m *mockMQTTMessage) ackCount() int32 {
	return atomic.LoadInt32(&m.acked)
}

type mockMQTTClient struct {
	mu           sync.Mutex
	subscribeErr error
	handler      mqtt.MessageHandler
	subscribed   chan struct{}
}

func newMockMQTTClient() *mockMQTTClient {
	return &mockMQTTClient{subscribed: make(chan struct{})}
}

func (m *mockMQTTClient) Subscribe(topic string, handler mqtt.MessageHandler) error {
	if m.subscribeErr != nil {
		return m.subscribeErr
	}

	m.mu.Lock()
	m.handler = handler
	m.mu.Unlock()
	close(m.subscribed)

	return nil
}

func (m *mockMQTTClient) deliver(msg mqtt.Message) {
	m.mu.Lock()
	handler := m.handler
	m.mu.Unlock()
	handler(nil, msg)
}

func (m *mockMQTTClient) Publish(ctx context.Context, topic string, payload []byte) error {
	return nil
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.After(timeout)
	for !cond() {
		select {
		case <-deadline:
			t.Fatal(msg)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestMQTTBroker_AcksOnSuccess(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	milvusCli := &mockMilvusClient{}
	mqttCli := newMockMQTTClient()
	mb := NewMQTTBroker(mqttCli, []IMilvusClientInterface{milvusCli})

	startDone := make(chan error, 1)
	go func() {
		startDone <- mb.Start("topic", Queue)
	}()

	select {
	case <-mqttCli.subscribed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscribe")
	}

	msg := &mockMQTTMessage{payload: raw}
	mqttCli.deliver(msg)

	waitFor(t, 2*time.Second, func() bool { return msg.ackCount() == 1 }, "timed out waiting for ack")

	mb.Stop()

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

func TestMQTTBroker_DoesNotAckOnFailure(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	permanentErr := errors.New("permanent")
	milvusCli := &mockMilvusClient{insertErrs: []error{permanentErr, permanentErr, permanentErr}}
	mqttCli := newMockMQTTClient()
	mb := NewMQTTBroker(mqttCli, []IMilvusClientInterface{milvusCli}, WithMQTTRetryDelay(time.Millisecond))

	go func() {
		_ = mb.Start("topic", Queue)
	}()

	select {
	case <-mqttCli.subscribed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscribe")
	}

	msg := &mockMQTTMessage{payload: raw}
	mqttCli.deliver(msg)

	waitFor(t, 2*time.Second, func() bool { return milvusCli.calls() == MaxHandleRetries }, "timed out waiting for retries to exhaust")

	mb.Stop()

	if msg.ackCount() != 0 {
		t.Fatalf("expected the message to remain unacked, got %d acks", msg.ackCount())
	}
}

func TestMQTTBroker_Start_RejectsNonQueuePattern(t *testing.T) {
	mb := NewMQTTBroker(newMockMQTTClient(), nil)

	err := mb.Start("topic", PubSub)
	if err == nil {
		t.Fatal("expected error for unsupported pattern")
	}
}

func TestMQTTBroker_Start_PropagatesSubscribeError(t *testing.T) {
	subscribeErr := errors.New("connection refused")
	mqttCli := &mockMQTTClient{subscribeErr: subscribeErr}
	mb := NewMQTTBroker(mqttCli, nil)

	err := mb.Start("topic", Queue)
	if !errors.Is(err, subscribeErr) {
		t.Fatalf("expected %v, got %v", subscribeErr, err)
	}
}

func TestMQTTBroker_Stop_Idempotent(t *testing.T) {
	mb := NewMQTTBroker(newMockMQTTClient(), nil)

	mb.Stop()
	mb.Stop()

	select {
	case <-mb.done:
	default:
		t.Fatal("expected done channel to be closed after Stop")
	}
}
