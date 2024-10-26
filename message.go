package milvus_cdc

type MessageCDC struct {
	Action         string `json:"action"`
	Vector         string `json:"vector"`
	CollectionName string `json:"collection_name"`
	PartitionTag   string `json:"partition_tag"`
	ExtraParams    string `json:"extra_params"`
	Id             int64  `json:"id"`
	Dimension      int64  `json:"dimension"`
	IndexFileSize  int64  `json:"index_file_size"`
	IndexType      int64  `json:"index_type"`
	MetricType     int32  `json:"metric_type"`
}
