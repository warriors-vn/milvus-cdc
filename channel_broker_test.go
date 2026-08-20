package milvus_cdc

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestChannelBroker_ProcessesMessageAndStops(t *testing.T) {
	raw, _ := json.Marshal(&MessageCDC{Action: Insert, CollectionName: "c", Id: 1})

	msgCh := make(chan string, 1)
	msgCh <- string(raw)

	milvusCli := &mockMilvusClient{}

	var mu sync.Mutex
	var gotErr error
	processed := make(chan struct{})

	cb := NewChannelBroker(msgCh, []IMilvusClientInterface{milvusCli}, WithChannelOnMessage(func(msg string, idx int, err error) {
		mu.Lock()
		gotErr = err
		mu.Unlock()
		close(processed)
	}))

	startDone := make(chan error, 1)
	go func() {
		startDone <- cb.Start("", "")
	}()

	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message to be processed")
	}

	cb.Stop()

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

func TestChannelBroker_StopsWhenChannelClosed(t *testing.T) {
	msgCh := make(chan string)
	close(msgCh)

	cb := NewChannelBroker(msgCh, []IMilvusClientInterface{&mockMilvusClient{}})

	startDone := make(chan error, 1)
	go func() {
		startDone <- cb.Start("", "")
	}()

	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("expected Start to return nil when msgCh is closed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Start to return after channel close")
	}
}

func TestChannelBroker_Stop_Idempotent(t *testing.T) {
	cb := NewChannelBroker(make(chan string), nil)

	cb.Stop()
	cb.Stop()

	select {
	case <-cb.done:
	default:
		t.Fatal("expected done channel to be closed after Stop")
	}
}
