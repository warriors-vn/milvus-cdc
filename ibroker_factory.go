package milvus_cdc

type IBrokerFactory interface {
	Start(channel string) error
}
