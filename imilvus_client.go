package milvus_cdc

type IMilvusClientInterface interface {
	Insert(vector, collectionName, partitionTag string, id int64) error
	Delete(collectionName, partitionTag string, id int64) error
	DropCollection(collectionName string) error
	CreateCollection(collectionName string, dimension, indexSize int64, metric int32) error
	CreateIndex(collectionName, extraParams string, indexType int64) error
	DropIndex(collectionName string) error
	CreatePartition(collectionName, partitionTag string) error
	DropPartition(collectionName, partitionTag string) error
}
