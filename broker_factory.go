package milvus_cdc

import (
	"fmt"
)

type BrokerFactory struct {
	brokers map[string]IBrokerFactory
}

// NewBrokerFactory registers a set of brokers by name, e.g.
// NewBrokerFactory(map[string]IBrokerFactory{Redis: redisBroker, GoChannel: channelBroker}).
func NewBrokerFactory(brokers map[string]IBrokerFactory) *BrokerFactory {
	return &BrokerFactory{
		brokers: brokers,
	}
}

func (bf *BrokerFactory) GetBrokerFactory(name string) (IBrokerFactory, error) {
	broker, ok := bf.brokers[name]
	if !ok {
		return nil, fmt.Errorf("the broker is invaild")
	}

	return broker, nil
}
