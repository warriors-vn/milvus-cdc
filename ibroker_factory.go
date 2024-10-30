package milvus_cdc

type IBrokerFactory interface {
	Start(channel, pattern string) error
	Stop()
}
