package milvus_cdc

import "time"

const (
	Redis = "redis"
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
)

const (
	DefaultTimeout = 10 * time.Second
)
