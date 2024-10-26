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
	DefaultTimeout = 10 * time.Second
)
