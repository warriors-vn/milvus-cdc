package milvus_cdc

import (
	"fmt"
)

type BrokerFactory struct {
	redisBroker *RedisBroker
	channel     string
}

func NewBrokerFactory(channel string, redisBroker *RedisBroker) *BrokerFactory {
	return &BrokerFactory{
		channel:     channel,
		redisBroker: redisBroker,
	}
}

func (bf *BrokerFactory) GetBrokerFactory(name string) (IBrokerFactory, error) {
	switch name {
	case Redis:
		return bf.redisBroker, nil
	}

	return nil, fmt.Errorf("the broker is invaild")
}
