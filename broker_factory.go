package milvus_cdc

import (
	"fmt"
)

type BrokerFactory struct {
	redisBroker *RedisBroker
}

func NewBrokerFactory(redisBroker *RedisBroker) *BrokerFactory {
	return &BrokerFactory{
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
