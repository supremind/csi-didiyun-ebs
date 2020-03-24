package ebs

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerServer(t *testing.T) {
	nodeID := "test-node"
	ebsClient := newMockEbsClient()
	driver := csicommon.NewCSIDriver(driverName, csiVersion, nodeID)
	require.NotNil(t, driver)
	svr := NewControllerServer(driver, ebsClient)
	ctx := context.Background()
	createReq := &csi.CreateVolumeRequest{
		Name:          "test-vol",
		CapacityRange: &csi.CapacityRange{RequiredBytes: 10000},
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessType: &csi.VolumeCapability_Mount{}},
		},
		Parameters: map[string]string{"k1": "v1"},
	}
	createResp, e := svr.CreateVolume(ctx, createReq)
	if assert.NoError(t, e) {
		if assert.NotNil(t, createResp.GetVolume()) {
			assert.Equal(t, createReq.GetCapacityRange().GetRequiredBytes(), createResp.GetVolume().GetCapacityBytes())
			assert.Equal(t, createReq.GetParameters(), createResp.GetVolume().GetVolumeContext())
		}
	}

	pubReq := &csi.ControllerPublishVolumeRequest{
		VolumeId: createResp.GetVolume().GetVolumeId(),
		NodeId:   nodeID,
	}
	_, e = svr.ControllerPublishVolume(ctx, pubReq)
	assert.NoError(t, e)

	unpubReq := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: createResp.GetVolume().GetVolumeId(),
		NodeId:   nodeID,
	}
	_, e = svr.ControllerUnpublishVolume(ctx, unpubReq)
	assert.NoError(t, e)

	delReq := &csi.DeleteVolumeRequest{VolumeId: createResp.GetVolume().GetVolumeId()}
	_, e = svr.DeleteVolume(ctx, delReq)
	assert.NoError(t, e)
}
