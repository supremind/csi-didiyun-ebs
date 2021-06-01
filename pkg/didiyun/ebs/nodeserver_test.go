package ebs

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	didiyunClient "github.com/supremind/didiyun-client/pkg"
	"k8s.io/kubernetes/pkg/util/mount"
)

func TestNodeServer(t *testing.T) {
	c, _ := didiyunClient.NewMock()
	nodeID := "test-node"
	nodeIP := "10.1.1.2"
	driver := csicommon.NewCSIDriver(driverName, csiVersion, nodeID)
	require.NotNil(t, driver)

	svr := &nodeServer{
		nodeID:            nodeID,
		nodeIP:            nodeIP,
		zone:              "zone1",
		mounter:           &mount.FakeMounter{},
		DefaultNodeServer: csicommon.NewDefaultNodeServer(driver),
		ebsCli:            c.Ebs(),
	}
	ctx := context.Background()
	volID, e := svr.ebsCli.Create(ctx, "", "zone1", "test-vol", "", 1000000)
	require.NoError(t, e)

	tmp, e := ioutil.TempDir("", "ebs_nodeserver_test-")
	require.NoError(t, e)
	defer func() {
		_ = os.RemoveAll(tmp)
	}()
	stagePath := filepath.Join(tmp, "stage")
	e = os.MkdirAll(stagePath, 0755)
	require.NoError(t, e)
	targetPath := filepath.Join(tmp, "target")
	e = os.MkdirAll(targetPath, 0755)
	require.NoError(t, e)

	volCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
		},
	}
	stgReq := &csi.NodeStageVolumeRequest{
		VolumeId:          volID,
		StagingTargetPath: stagePath,
		VolumeCapability:  volCap,
	}
	_, e = svr.NodeStageVolume(ctx, stgReq)
	assert.NoError(t, e)

	pubReq := &csi.NodePublishVolumeRequest{
		VolumeId:          volID,
		StagingTargetPath: stagePath,
		TargetPath:        targetPath,
		VolumeCapability:  volCap,
	}
	_, e = svr.NodePublishVolume(ctx, pubReq)
	assert.NoError(t, e)

	unpubReq := &csi.NodeUnpublishVolumeRequest{
		VolumeId:   volID,
		TargetPath: targetPath,
	}
	_, e = svr.NodeUnpublishVolume(ctx, unpubReq)
	assert.NoError(t, e)

	unstgReq := &csi.NodeUnstageVolumeRequest{
		VolumeId:          volID,
		StagingTargetPath: stagePath,
	}
	_, e = svr.NodeUnstageVolume(ctx, unstgReq)
	assert.NoError(t, e)
}
