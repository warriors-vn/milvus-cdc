package milvus_cdc

type MessageCDC struct {
	action string
	vector string
	id     int64
}
