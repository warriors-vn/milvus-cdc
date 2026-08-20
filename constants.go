package milvus_cdc

import "time"

const (
	Redis     = "redis"
	GoChannel = "go-channel"
	RabbitMQ  = "rabbitmq"
	MQTT      = "mqtt"
	Kafka     = "kafka"
)

const (
	Insert           = "insert"
	Delete           = "delete"
	CreateCollection = "create-collection"
	DropCollection   = "drop-collection"
	CreatePartition  = "create-partition"
	DropPartition    = "drop-partition"
	CreateIndex      = "create-index"
	DropIndex        = "drop-index"
)

const (
	PubSub = "pub-sub"
	Queue  = "queue"
	Stream = "stream"
)

const (
	// DefaultStreamGroup is the consumer group RedisBroker joins when
	// WithStreamGroup isn't set.
	DefaultStreamGroup = "milvus-cdc"
	// StreamPayloadField is the field name RedisBroker reads/writes the raw CDC
	// message under in a stream entry (see RedisClient.XAdd).
	StreamPayloadField = "payload"
)

const (
	DefaultTimeout = 10 * time.Second
)

const (
	MaxHandleRetries = 3
	HandleRetryDelay = 500 * time.Millisecond
)
