package milvus_cdc

import "github.com/milvus-io/milvus-sdk-go/milvus"

type IMilvusClientInterface interface {
	Insert(vector, collectionName, partitionTag string, id int64) error
	Delete(collectionName, partitionTag string, id int64) error
	DropCollection(collectionName string) error
	CreateCollection(collectionName string, dimension, indexSize int64, metric milvus.MetricType) error
	CreateIndex(collectionName string, nList int64, indexType milvus.IndexType) error
	DropIndex(collectionName string) error
	CreatePartition(collectionName, partitionTag string) error
	DropPartition(collectionName, partitionTag string) error
}
