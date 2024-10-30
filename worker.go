package milvus_cdc

type WorkerCDC struct {
	brokerFactory *BrokerFactory
}

func NewWorkerCDC(brokerFactory *BrokerFactory) *WorkerCDC {
	return &WorkerCDC{
		brokerFactory: brokerFactory,
	}
}

func (w *WorkerCDC) Start(broker, channel, pattern string) error {
	processor, err := w.brokerFactory.GetBrokerFactory(broker)
	if err != nil {
		return err
	}

	return processor.Start(channel, pattern)
}

func (w *WorkerCDC) Stop(broker string) error {
	processor, err := w.brokerFactory.GetBrokerFactory(broker)
	if err != nil {
		return err
	}

	processor.Stop()
	return nil
}
