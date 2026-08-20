package milvus_cdc

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// dispatcher parses raw CDC messages and applies them to a set of milvus clients, with
// per-message retry and an optional callback. It holds the logic shared by every
// broker implementation (RedisBroker, ChannelBroker, ...); a broker only owns the
// transport (how messages arrive), not what happens to them once they do.
type dispatcher struct {
	milvus     []IMilvusClientInterface
	onMessage  OnMessageFunc
	maxRetries int
	retryDelay time.Duration
}

func newDispatcher(milvus []IMilvusClientInterface) dispatcher {
	return dispatcher{
		milvus:     milvus,
		maxRetries: MaxHandleRetries,
		retryDelay: HandleRetryDelay,
	}
}

func (d *dispatcher) reportMessage(msg string, idx int, err error) {
	if d.onMessage != nil {
		d.onMessage(msg, idx, err)
	}
}

// broadcast delivers msg to every milvus client concurrently, waits for all of them to
// finish (each with its own retry), and reports the outcome of each via onMessage. It
// returns a joined error of every client's failure (nil if all succeeded), which a
// broker with delivery acknowledgement (e.g. RabbitMQBroker) can use to decide whether
// to ack or nack the message.
func (d *dispatcher) broadcast(msg string) error {
	var wg sync.WaitGroup
	errs := make([]error, len(d.milvus))
	for i := range d.milvus {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := d.handle(msg, idx)
			errs[idx] = err
			d.reportMessage(msg, idx, err)
		}(i)
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (d *dispatcher) handle(msg string, idx int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic while handling message: %v", r)
		}
	}()

	var message MessageCDC

	errUnmarshal := json.Unmarshal([]byte(msg), &message)
	if errUnmarshal != nil {
		return errUnmarshal
	}

	for attempt := 1; attempt <= d.maxRetries; attempt++ {
		err = d.sync(&message, idx)
		if err == nil {
			return nil
		}

		if attempt < d.maxRetries {
			time.Sleep(d.retryDelay)
		}
	}

	return err
}

func (d *dispatcher) sync(message *MessageCDC, idx int) error {
	if message == nil {
		return fmt.Errorf("message cdc not found")
	}

	if len(d.milvus) <= idx {
		return fmt.Errorf("milvus client not found")
	}

	switch message.Action {
	case Insert:
		return d.milvus[idx].Insert(message.Vector, message.CollectionName, message.PartitionTag, message.Id)
	case Delete:
		return d.milvus[idx].Delete(message.CollectionName, message.PartitionTag, message.Id)
	case CreateCollection:
		return d.milvus[idx].CreateCollection(message.CollectionName, message.Dimension, message.IndexFileSize, message.MetricType)
	case DropCollection:
		return d.milvus[idx].DropCollection(message.CollectionName)
	case CreatePartition:
		return d.milvus[idx].CreatePartition(message.CollectionName, message.PartitionTag)
	case DropPartition:
		return d.milvus[idx].DropPartition(message.CollectionName, message.PartitionTag)
	case CreateIndex:
		return d.milvus[idx].CreateIndex(message.CollectionName, message.NList, message.IndexType)
	case DropIndex:
		return d.milvus[idx].DropIndex(message.CollectionName)
	}

	return fmt.Errorf("the action is invalid")
}
