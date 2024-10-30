package milvus_cdc

type IWorkerInterface interface {
	Start(broker, channel, pattern string) error
	Stop(broker string) error
}
