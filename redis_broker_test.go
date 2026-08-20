package milvus_cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

type mockRedisClient struct {
	mu              sync.Mutex
	brpopSeq        [][]string
	calls           int
	xGroupCreateErr error
	xReadGroupSeq   [][]redis.XMessage
	xReadGroupIdx   int
	xAcked          []string
}

func (m *mockRedisClient) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return nil
}

func (m *mockRedisClient) Publish(ctx context.Context, channel, message string) (int64, error) {
	return 0, nil
}

func (m *mockRedisClient) LPush(ctx context.Context, queue string, value interface{}) (int64, error) {
	return 0, nil
}

func (m *mockRedisClient) BRPop(ctx context.Context, queue string, timeout time.Duration) ([]string, error) {
	m.mu.Lock()
	if m.calls < len(m.brpopSeq) {
		res := m.brpopSeq[m.calls]
		m.calls++
		m.mu.Unlock()
		return res, nil
	}
	m.mu.Unlock()

	// mimic real BRPop with timeout 0: block until the caller cancels the context.
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockRedisClient) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	return m.xGroupCreateErr
}

func (m *mockRedisClient) XReadGroup(ctx context.Context, group, consumer, stream string, block time.Duration) ([]redis.XMessage, error) {
	m.mu.Lock()
	if m.xReadGroupIdx < len(m.xReadGroupSeq) {
		res := m.xReadGroupSeq[m.xReadGroupIdx]
		m.xReadGroupIdx++
		m.mu.Unlock()
		return res, nil
	}
	m.mu.Unlock()

	// mimic real XReadGroup with block 0: block until the caller cancels the context.
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockRedisClient) XAck(ctx context.Context, stream, group string, ids ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.xAcked = append(m.xAcked, ids...)
	return nil
}

func (m *mockRedisClient) XAdd(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	return "", nil
}

func (m *mockRedisClient) ackedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.xAcked)
}

func TestRedisBroker_Stop_Idempotent(t *testing.T) {
	rb := NewRedisBroker(&mockRedisClient{}, nil)

	rb.Stop()
	rb.Stop()

	select {
	case <-rb.done:
	default:
		t.Fatal("expected done channel to be closed after Stop")
	}
}

func TestRedisBroker_Queue_ProcessesMessageAndStops(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	redisCli := &mockRedisClient{brpopSeq: [][]string{{"channel", string(raw)}}}
	milvusCli := &mockMilvusClient{}

	var mu sync.Mutex
	var gotErr error
	processed := make(chan struct{})

	rb := NewRedisBroker(redisCli, []IMilvusClientInterface{milvusCli}, WithOnMessage(func(msg string, idx int, err error) {
		mu.Lock()
		gotErr = err
		mu.Unlock()
		close(processed)
	}))

	startDone := make(chan error, 1)
	go func() {
		startDone <- rb.Start("channel", Queue)
	}()

	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message to be processed")
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

	mu.Lock()
	defer mu.Unlock()
	if gotErr != nil {
		t.Fatalf("expected message to be processed successfully, got %v", gotErr)
	}
	if milvusCli.calls() != 1 {
		t.Fatalf("expected 1 insert call, got %d", milvusCli.calls())
	}
}

func TestRedisBroker_Start_InvalidPattern(t *testing.T) {
	rb := NewRedisBroker(&mockRedisClient{}, nil)

	err := rb.Start("channel", "not-a-real-pattern")
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

func TestRedisBroker_Stream_AcksOnSuccess(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	redisCli := &mockRedisClient{
		xReadGroupSeq: [][]redis.XMessage{
			{{ID: "1-0", Values: map[string]interface{}{StreamPayloadField: string(raw)}}},
		},
	}
	milvusCli := &mockMilvusClient{}

	var mu sync.Mutex
	var gotErr error
	processed := make(chan struct{})

	rb := NewRedisBroker(redisCli, []IMilvusClientInterface{milvusCli}, WithOnMessage(func(msg string, idx int, err error) {
		mu.Lock()
		gotErr = err
		mu.Unlock()
		close(processed)
	}))

	startDone := make(chan error, 1)
	go func() {
		startDone <- rb.Start("stream", Stream)
	}()

	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message to be processed")
	}

	waitFor(t, 2*time.Second, func() bool { return redisCli.ackedCount() == 1 }, "timed out waiting for XAck")

	rb.Stop()

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

func TestRedisBroker_Stream_DoesNotAckOnFailure(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	permanentErr := errors.New("permanent")
	redisCli := &mockRedisClient{
		xReadGroupSeq: [][]redis.XMessage{
			{{ID: "1-0", Values: map[string]interface{}{StreamPayloadField: string(raw)}}},
		},
	}
	milvusCli := &mockMilvusClient{insertErrs: []error{permanentErr, permanentErr, permanentErr}}

	rb := NewRedisBroker(redisCli, []IMilvusClientInterface{milvusCli}, WithRetryDelay(time.Millisecond))

	go func() {
		_ = rb.Start("stream", Stream)
	}()

	waitFor(t, 2*time.Second, func() bool { return milvusCli.calls() == MaxHandleRetries }, "timed out waiting for retries to exhaust")

	rb.Stop()

	if redisCli.ackedCount() != 0 {
		t.Fatalf("expected no acks, got %d", redisCli.ackedCount())
	}
}

func TestRedisBroker_Stream_PropagatesGroupCreateError(t *testing.T) {
	groupErr := errors.New("connection refused")
	redisCli := &mockRedisClient{xGroupCreateErr: groupErr}
	rb := NewRedisBroker(redisCli, nil)

	err := rb.Start("stream", Stream)
	if !errors.Is(err, groupErr) {
		t.Fatalf("expected %v, got %v", groupErr, err)
	}
}
