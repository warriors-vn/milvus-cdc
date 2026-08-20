package milvus_cdc

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/milvus-io/milvus-sdk-go/milvus"
)

type mockMilvusClient struct {
	mu          sync.Mutex
	insertErrs  []error
	insertCalls int
}

func (m *mockMilvusClient) Insert(vector, collectionName, partitionTag string, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.insertCalls
	m.insertCalls++
	if idx < len(m.insertErrs) {
		return m.insertErrs[idx]
	}

	return nil
}

func (m *mockMilvusClient) Delete(collectionName, partitionTag string, id int64) error {
	return nil
}

func (m *mockMilvusClient) DropCollection(collectionName string) error {
	return nil
}

func (m *mockMilvusClient) CreateCollection(collectionName string, dimension, indexSize int64, metric milvus.MetricType) error {
	return nil
}

func (m *mockMilvusClient) CreateIndex(collectionName string, nList int64, indexType milvus.IndexType) error {
	return nil
}

func (m *mockMilvusClient) DropIndex(collectionName string) error {
	return nil
}

func (m *mockMilvusClient) CreatePartition(collectionName, partitionTag string) error {
	return nil
}

func (m *mockMilvusClient) DropPartition(collectionName, partitionTag string) error {
	return nil
}

func (m *mockMilvusClient) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.insertCalls
}

type panicMilvusClient struct {
	mockMilvusClient
}

func (p *panicMilvusClient) Insert(vector, collectionName, partitionTag string, id int64) error {
	p.mu.Lock()
	p.insertCalls++
	p.mu.Unlock()

	panic("boom")
}

func TestDispatcher_Sync_Insert(t *testing.T) {
	milvusCli := &mockMilvusClient{}
	d := newDispatcher([]IMilvusClientInterface{milvusCli})

	err := d.sync(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if milvusCli.calls() != 1 {
		t.Fatalf("expected 1 insert call, got %d", milvusCli.calls())
	}
}

func TestDispatcher_Sync_InvalidAction(t *testing.T) {
	d := newDispatcher([]IMilvusClientInterface{&mockMilvusClient{}})

	err := d.sync(&MessageCDC{Action: "unknown"}, 0)
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestDispatcher_Sync_IndexOutOfRange(t *testing.T) {
	d := newDispatcher([]IMilvusClientInterface{&mockMilvusClient{}})

	err := d.sync(&MessageCDC{Action: Insert}, 5)
	if err == nil {
		t.Fatal("expected error for out-of-range milvus index")
	}
}

func TestDispatcher_Handle_RetriesTransientFailureThenSucceeds(t *testing.T) {
	milvusCli := &mockMilvusClient{insertErrs: []error{errors.New("transient"), errors.New("transient")}}
	d := newDispatcher([]IMilvusClientInterface{milvusCli})
	d.retryDelay = time.Millisecond

	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	err := d.handle(string(raw), 0)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if milvusCli.calls() != 3 {
		t.Fatalf("expected 3 insert attempts, got %d", milvusCli.calls())
	}
}

func TestDispatcher_Handle_GivesUpAfterMaxRetries(t *testing.T) {
	wantErr := errors.New("permanent")
	milvusCli := &mockMilvusClient{insertErrs: []error{wantErr, wantErr, wantErr}}
	d := newDispatcher([]IMilvusClientInterface{milvusCli})
	d.retryDelay = time.Millisecond

	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	err := d.handle(string(raw), 0)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	if milvusCli.calls() != MaxHandleRetries {
		t.Fatalf("expected %d insert attempts, got %d", MaxHandleRetries, milvusCli.calls())
	}
}

func TestDispatcher_Handle_DoesNotRetryOnMalformedMessage(t *testing.T) {
	milvusCli := &mockMilvusClient{}
	d := newDispatcher([]IMilvusClientInterface{milvusCli})
	d.retryDelay = time.Millisecond

	err := d.handle("not-json", 0)
	if err == nil {
		t.Fatal("expected error for malformed message")
	}
	if milvusCli.calls() != 0 {
		t.Fatalf("expected 0 insert attempts for a malformed message, got %d", milvusCli.calls())
	}
}

func TestDispatcher_Handle_RecoversFromPanic(t *testing.T) {
	milvusCli := &panicMilvusClient{}
	d := newDispatcher([]IMilvusClientInterface{milvusCli})

	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	err := d.handle(string(raw), 0)
	if err == nil {
		t.Fatal("expected error recovered from panic")
	}
}

func TestDispatcher_Broadcast_ReportsEveryClient(t *testing.T) {
	clientA := &mockMilvusClient{}
	clientB := &mockMilvusClient{insertErrs: []error{errors.New("boom"), errors.New("boom"), errors.New("boom")}}
	d := newDispatcher([]IMilvusClientInterface{clientA, clientB})
	d.retryDelay = time.Millisecond

	var mu sync.Mutex
	results := make(map[int]error)
	d.onMessage = func(msg string, idx int, err error) {
		mu.Lock()
		results[idx] = err
		mu.Unlock()
	}

	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})
	d.broadcast(string(raw))

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0] != nil {
		t.Fatalf("expected client 0 to succeed, got %v", results[0])
	}
	if results[1] == nil {
		t.Fatal("expected client 1 to fail after exhausting retries")
	}
}
