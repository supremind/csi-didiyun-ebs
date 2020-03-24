package dataset

import (
	"io"

	"git.supremind.info/products/atom/proto/go/api"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type mockDsClient struct {
	data      chan *api.DataItem
	volBucket string
	volPath   string
}

func (t *mockDsClient) datasetService(endpoint, user string) (api.DatasetServiceClient, error) {
	return &mockDatasetServiceClient{data: t.data}, nil
}

func (t *mockDsClient) volumeService(endpoint, user string) (api.VolumeServiceClient, error) {
	return &mockVolumeServiceClient{volBucket: t.volBucket, volPath: t.volPath}, nil
}

func (t *mockDsClient) close() {
}

type mockDatasetServiceClient struct {
	data chan *api.DataItem
	api.DatasetServiceClient
}

func (t *mockDatasetServiceClient) ListDataItems(ctx context.Context, in *api.ListDataItemsReq, opts ...grpc.CallOption) (api.DatasetService_ListDataItemsClient, error) {
	return &mockDatasetService_ListDataItemsClient{data: t.data}, nil
}

type mockDatasetService_ListDataItemsClient struct {
	data chan *api.DataItem
	grpc.ClientStream
}

func (t *mockDatasetService_ListDataItemsClient) Recv() (*api.DataItem, error) {
	item := <-t.data
	if item == nil {
		return nil, io.EOF
	}
	return item, nil
}

type mockVolumeServiceClient struct {
	volBucket string
	volPath   string
	api.VolumeServiceClient
}

func (t *mockVolumeServiceClient) GetVolume(ctx context.Context, in *api.GetVolumeReq, opts ...grpc.CallOption) (*api.Volume, error) {
	return &api.Volume{Spec: &api.VolumeSpec{ResourceVolume: &api.ResourceVolume{Bucket: t.volBucket, Path: t.volPath}}}, nil
}
