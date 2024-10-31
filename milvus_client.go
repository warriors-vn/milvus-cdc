package milvus_cdc

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/milvus-io/milvus-sdk-go/milvus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type MilvusClient struct {
	milvus  milvus.MilvusClient
	timeout time.Duration
}

func NewMilvusClient(host, port string, timeout time.Duration) (*MilvusClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	opts := grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                DefaultTimeout,
		Timeout:             DefaultTimeout,
		PermitWithoutStream: false,
	})

	connectParam := milvus.ConnectParam{
		IPAddress: host,
		Port:      port,
		Opts:      []grpc.DialOption{opts},
	}

	client, err := milvus.NewMilvusClient(ctx, connectParam)
	if err != nil {
		return nil, err
	}

	isConnected := client.IsConnected(ctx)
	if !isConnected {
		return nil, fmt.Errorf("milvus is not connected")
	}

	return &MilvusClient{
		milvus:  client,
		timeout: timeout,
	}, nil
}

func (mc *MilvusClient) Insert(vector, collectionName, partitionTag string, id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	vByte, err := hex.DecodeString(vector)
	if err != nil {
		return err
	}

	_, status, err := mc.milvus.Insert(ctx, &milvus.InsertParam{
		CollectionName: collectionName,
		PartitionTag:   partitionTag,
		RecordArray: []milvus.Entity{
			{
				FloatData: DecodeUnsafeF32(vByte),
			},
		},
		IDArray: []int64{id},
	})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (mc *MilvusClient) Delete(collectionName, partitionTag string, id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	status, err := mc.milvus.DeleteEntityByID(ctx, collectionName, partitionTag, []int64{id})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (mc *MilvusClient) DropCollection(collectionName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	status, err := mc.milvus.DropCollection(ctx, collectionName)
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (mc *MilvusClient) CreateCollection(collectionName string, dimension, indexSize int64, metric milvus.MetricType) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	status, err := mc.milvus.CreateCollection(ctx, milvus.CollectionParam{
		CollectionName: collectionName,
		Dimension:      dimension,
		IndexFileSize:  indexSize,
		MetricType:     int32(metric),
	})

	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (mc *MilvusClient) CreateIndex(collectionName string, nList int64, indexType milvus.IndexType) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	indexParam := &milvus.IndexParam{
		CollectionName: collectionName,
		IndexType:      indexType,
		ExtraParams:    fmt.Sprintf("{\"nlist\" : %d}", nList),
	}

	status, err := mc.milvus.CreateIndex(ctx, indexParam)
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (mc *MilvusClient) DropIndex(collectionName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	status, err := mc.milvus.DropIndex(ctx, collectionName)
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (mc *MilvusClient) CreatePartition(collectionName, partitionTag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	status, err := mc.milvus.CreatePartition(ctx, milvus.PartitionParam{
		CollectionName: collectionName,
		PartitionTag:   partitionTag,
	})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}

func (mc *MilvusClient) DropPartition(collectionName, partitionTag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mc.timeout)
	defer cancel()

	status, err := mc.milvus.DropPartition(ctx, milvus.PartitionParam{
		CollectionName: collectionName,
		PartitionTag:   partitionTag,
	})
	if err != nil {
		return err
	}

	if !status.Ok() {
		return fmt.Errorf("%v", status.GetMessage())
	}

	return nil
}
