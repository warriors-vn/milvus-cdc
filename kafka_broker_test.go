package milvus_cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

type mockKafkaClient struct {
	mu        sync.Mutex
	messages  []kafka.Message
	idx       int
	committed []kafka.Message
}

func (m *mockKafkaClient) FetchMessage(ctx context.Context) (kafka.Message, error) {
	m.mu.Lock()
	if m.idx < len(m.messages) {
		msg := m.messages[m.idx]
		m.idx++
		m.mu.Unlock()
		return msg, nil
	}
	m.mu.Unlock()

	// mimic a real Reader blocking until the caller cancels the context.
	<-ctx.Done()
	return kafka.Message{}, ctx.Err()
}

func (m *mockKafkaClient) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.committed = append(m.committed, msgs...)
	return nil
}

func (m *mockKafkaClient) Publish(ctx context.Context, key, value []byte) error {
	return nil
}

func (m *mockKafkaClient) committedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.committed)
}

func TestKafkaBroker_CommitsOnSuccess(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	kafkaCli := &mockKafkaClient{messages: []kafka.Message{{Value: raw, Offset: 1}}}
	milvusCli := &mockMilvusClient{}

	var mu sync.Mutex
	var gotErr error
	processed := make(chan struct{})

	kb := NewKafkaBroker(kafkaCli, []IMilvusClientInterface{milvusCli}, WithKafkaOnMessage(func(msg string, idx int, err error) {
		mu.Lock()
		gotErr = err
		mu.Unlock()
		close(processed)
	}))

	startDone := make(chan error, 1)
	go func() {
		startDone <- kb.Start("topic", Queue)
	}()

	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message to be processed")
	}

	waitFor(t, 2*time.Second, func() bool { return kafkaCli.committedCount() == 1 }, "timed out waiting for commit")

	kb.Stop()

	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("expected Start to return nil after Stop, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Start to return after Stop")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotErr != nil {
		t.Fatalf("expected message to be processed successfully, got %v", gotErr)
	}
	if milvusCli.calls() != 1 {
		t.Fatalf("expected 1 insert call, got %d", milvusCli.calls())
	}
}

func TestKafkaBroker_DoesNotCommitOnFailure(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	permanentErr := errors.New("permanent")
	kafkaCli := &mockKafkaClient{messages: []kafka.Message{{Value: raw, Offset: 1}}}
	milvusCli := &mockMilvusClient{insertErrs: []error{permanentErr, permanentErr, permanentErr}}

	kb := NewKafkaBroker(kafkaCli, []IMilvusClientInterface{milvusCli}, WithKafkaRetryDelay(time.Millisecond))

	go func() {
		_ = kb.Start("topic", Queue)
	}()

	waitFor(t, 2*time.Second, func() bool { return milvusCli.calls() == MaxHandleRetries }, "timed out waiting for retries to exhaust")

	kb.Stop()

	if kafkaCli.committedCount() != 0 {
		t.Fatalf("expected no commits, got %d", kafkaCli.committedCount())
	}
}

func TestKafkaBroker_Start_RejectsNonQueuePattern(t *testing.T) {
	kb := NewKafkaBroker(&mockKafkaClient{}, nil)

	err := kb.Start("topic", PubSub)
	if err == nil {
		t.Fatal("expected error for unsupported pattern")
	}
}

func TestKafkaBroker_Stop_Idempotent(t *testing.T) {
	kb := NewKafkaBroker(&mockKafkaClient{}, nil)

	kb.Stop()
	kb.Stop()

	select {
	case <-kb.done:
	default:
		t.Fatal("expected done channel to be closed after Stop")
	}
}
