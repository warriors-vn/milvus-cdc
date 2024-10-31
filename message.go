package milvus_cdc

import "github.com/milvus-io/milvus-sdk-go/milvus"

type MessageCDC struct {
	Action         string            `json:"action"`
	Vector         string            `json:"vector"`
	CollectionName string            `json:"collection_name"`
	PartitionTag   string            `json:"partition_tag"`
	NList          int64             `json:"n_list"`
	Id             int64             `json:"id"`
	Dimension      int64             `json:"dimension"`
	IndexFileSize  int64             `json:"index_file_size"`
	IndexType      milvus.IndexType  `json:"index_type"`
	MetricType     milvus.MetricType `json:"metric_type"`
}
