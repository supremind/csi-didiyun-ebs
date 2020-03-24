package dataset

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.supremind.info/products/atom/proto/go/api"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
)

func TestMount(t *testing.T) {
	inputData := make(chan *api.DataItem)
	server := &nodeServer{
		dsClient: &mockDsClient{
			data:      inputData,
			volBucket: "buc",
			volPath:   "text",
		},
	}

	labelStr := `{"url": "http://bucket/to/file"}`
	var label types.Struct
	if e := jsonpb.UnmarshalString(labelStr, &label); e != nil {
		t.Fatal(e)
	}
	testFile := "a file for test"
	testFilePath := "buc/text"
	testFileName := "test.txt"
	annoKey := "labelX"
	go func() {
		inputData <- &api.DataItem{
			Metas: []*api.DatumMeta{{
				VolumeRef:     &api.ResourceReference{Kind: api.ResourceKindVolume, Name: "test", Creator: "admin"},
				Key:           testFileName,
				ContentLength: uint64(len(testFile)),
			}},
			Annotations: map[string]*types.Struct{annoKey: &label},
		}
		close(inputData)
	}()

	tmp, e := ioutil.TempDir("", "nodeserver_test-")
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()
	t.Logf("running test at tmp dir %s", tmp)

	stagePath := filepath.Join(tmp, "stage")
	if e := os.MkdirAll(stagePath, 0755); e != nil {
		t.Fatal(e)
	}
	targetPath := filepath.Join(tmp, driverNameCSI, "target")
	if e := os.MkdirAll(targetPath, 0755); e != nil {
		t.Fatal(e)
	}
	dataRootPath := filepath.Join(tmp, driverNameAlluxioFuse)
	if e := os.MkdirAll(filepath.Join(dataRootPath, testFilePath), 0755); e != nil {
		t.Fatal(e)
	}
	if e := ioutil.WriteFile(filepath.Join(dataRootPath, testFilePath, testFileName), []byte(testFile), 0600); e != nil {
		t.Fatal(e)
	}

	ctx := context.Background()
	volID := "test"
	cap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
	}
	volCtx := map[string]string{
		keyDatasetCreator:        "ava",
		keyDatasetName:           "test-ds",
		keyDatasetVersion:        "v1",
		keyDatasetIndexURIRegexp: `"url":"([^ ]+)"`,
		keyDatasetPath:           "/workspace/mnt/datasets/v1/test-ds/ava",
		keyDatasetIndexKey:       annoKey,
	}

	stgReq := &csi.NodeStageVolumeRequest{
		VolumeId:          volID,
		StagingTargetPath: stagePath,
		VolumeCapability:  cap,
		VolumeContext:     volCtx,
	}
	if _, e := server.NodeStageVolume(ctx, stgReq); e != nil {
		t.Fatal(e)
	}

	pubReq := &csi.NodePublishVolumeRequest{
		VolumeId:          volID,
		StagingTargetPath: stagePath,
		TargetPath:        targetPath,
		VolumeCapability:  cap,
		VolumeContext:     volCtx,
	}
	if _, e := server.NodePublishVolume(ctx, pubReq); e != nil {
		t.Fatal(e)
	}

	time.Sleep(1 * time.Second) // wait fuse server to start
	content, e := ioutil.ReadFile(filepath.Join(targetPath, testFilePath, testFileName))
	if e != nil {
		t.Fatal(e)
	}
	if g, e := string(content), testFile; g != e {
		t.Fatalf("wrong read results: %q != %q", g, e)
	}

	indexFile := `{"url":"/workspace/mnt/datasets/v1/test-ds/ava/buc/text/test.txt"}` + "\n"
	content, e = ioutil.ReadFile(filepath.Join(stagePath, datasetIndexDirectory, annoKey))
	if e != nil {
		t.Fatal(e)
	}
	if g, e := string(content), indexFile; g != e {
		t.Fatalf("wrong read results: %q != %q", g, e)
	}

	unpubReq := &csi.NodeUnpublishVolumeRequest{
		VolumeId:   volID,
		TargetPath: targetPath,
	}
	if _, e := server.NodeUnpublishVolume(ctx, unpubReq); e != nil {
		t.Fatal(e)
	}

	unstgReq := &csi.NodeUnstageVolumeRequest{
		VolumeId:          volID,
		StagingTargetPath: stagePath,
	}
	if _, e := server.NodeUnstageVolume(ctx, unstgReq); e != nil {
		t.Fatal(e)
	}

}
