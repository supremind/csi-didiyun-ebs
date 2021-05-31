package ebs

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	didiyunClient "github.com/supremind/didiyun-client/pkg"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	// might be overwritten by environment variable `MAX_VOLUMES_PER_NODE`
	defaultMaxVolumesPerNode = 5
	maxVolumePerNodeEnvKey   = "MAX_VOLUMES_PER_NODE"
)

type nodeServer struct {
	nodeID            string
	nodeIP            string
	region            string
	zone              string
	maxVolumesPerNode int64
	*csicommon.DefaultNodeServer
	mounter mount.Interface
	ebsCli  didiyunClient.EbsClient
}

func NewNodeServer(d *csicommon.CSIDriver, nodeID, nodeIP, region, zone string, cli didiyunClient.EbsClient) *nodeServer {
	var maxVolumesPerNode int64 = defaultMaxVolumesPerNode
	if val, e := strconv.ParseInt(os.Getenv(maxVolumePerNodeEnvKey), 10, 64); e != nil {
		klog.V(2).Infof("parse env var %s failed: %v", maxVolumePerNodeEnvKey, e)
	} else if val <= 0 {
		klog.V(2).Infof("invalid env var %s value: %d", maxVolumePerNodeEnvKey, val)
	} else {
		maxVolumesPerNode = val
	}

	return &nodeServer{
		nodeID:            nodeID,
		nodeIP:            nodeIP,
		region:            region,
		zone:              zone,
		maxVolumesPerNode: maxVolumesPerNode,
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		mounter:           mount.New(""),
		ebsCli:            cli,
	}
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	sourcePath := req.StagingTargetPath
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	if isBlock {
		// TODO: handle block volume
		return nil, status.Error(codes.Unimplemented, "")
	}
	targetPath := req.GetTargetPath()
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging Target Path cannot be emtpy")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capability cannot be emtpy")
	}

	notmounted, e := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	if !notmounted {
		klog.V(2).Infof("volume %s at path %s is already mounted", req.VolumeId, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// start to mount
	mnt := req.VolumeCapability.GetMount()
	options := append(mnt.MountFlags, "bind")
	if req.Readonly {
		options = append(options, "ro")
	}
	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	if e := ns.mounter.Mount(sourcePath, targetPath, fsType, options); e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}

	klog.V(4).Infof("mounted volume %s (%s -> %s) with flags %v and fsType %s", req.VolumeId, sourcePath, targetPath, options, fsType)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	notmounted, e := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	if notmounted {
		klog.V(2).Infof("volume %s at path %s is already unmounted", req.VolumeId, targetPath)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if e := ns.mounter.Unmount(targetPath); e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}

	klog.V(4).Infof("unmounted volume %s from %s", req.VolumeId, targetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	targetPath := req.StagingTargetPath
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging Target Path cannot be empty")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capability is null")
	}

	notmounted, e := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	if !notmounted {
		klog.V(2).Infof("volume %s at global path %s is already mounted", req.VolumeId, targetPath)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	isBlock := req.GetVolumeCapability().GetBlock() != nil
	if isBlock {
		// TODO: mount block device
		return nil, status.Error(codes.Unimplemented, "")
	}

	// check or attach
	var device string

	ebs, e := ns.ebsCli.Get(ctx, req.GetVolumeId())
	if e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	if ebs.GetDc2() != nil {
		if ebs.GetDc2().GetIp() != ns.nodeIP {
			msg := fmt.Sprintf("ebs %s (%s) is mounted to another node %s, could not be published to %s", ebs.GetName(), ebs.GetEbsUuid(), ebs.GetDc2().GetName(), ns.nodeID)
			klog.Errorf(msg)
			return nil, status.Error(codes.FailedPrecondition, msg)
		}

		device = ebs.GetDeviceName()
		klog.V(4).Infof("ebs %s (%s) is already mounted to %s as %s", ebs.GetName(), ebs.GetEbsUuid(), ns.nodeID, device)

	} else {
		// attach before mount to global
		var e error
		device, e = ns.ebsCli.Attach(ctx, req.GetVolumeId(), ns.nodeIP)
		if e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
		klog.V(4).Infof("ebs %s (%s) is mounted to %s as %s", ebs.GetName(), ebs.GetEbsUuid(), ns.nodeID, device)
	}

	var err error
	defer func() { // detach volume in case of any error, because NodeUnstageVolume won't be called
		if err != nil {
			if e := ns.ebsCli.Detach(ctx, req.GetVolumeId()); e != nil {
				klog.Errorf("volume %s, Device: %s, detach error: %s", req.GetVolumeId(), device, e)
			}
		}
	}()

	// mount
	mnt := req.VolumeCapability.GetMount()
	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}
	diskMounter := &mount.SafeFormatAndMount{Interface: ns.mounter, Exec: mount.NewOsExec()}
	if err = diskMounter.FormatAndMount("/dev/"+device, targetPath, fsType, mnt.MountFlags); err != nil {
		klog.Errorf("volume %s, Device: %s, FormatAndMount error: %s", req.GetVolumeId(), device, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Infof("staged volume %s, target %s, device: %s", req.GetVolumeId(), targetPath, device)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	targetPath := req.StagingTargetPath
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging Target Path can not be empty")
	}

	notmounted, e := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	if !notmounted {
		// check volume unattached before unmount for troubleshooting
		if os.Getenv("ENABLE_CHECK_DEVICE") != "" && os.Getenv("ENABLE_CHECK_DEVICE") != "0" {
			if e := checkDevice(targetPath); e != nil {
				klog.Errorf("check device failed for path %s of volume %s before umount: %s", targetPath, req.VolumeId, e)
				return nil, status.Error(codes.Internal, "device not found before umount")
			}
		}
		if e := ns.mounter.Unmount(targetPath); e != nil {
			return nil, status.Error(codes.Internal, e.Error())
		}
	} else {
		klog.V(2).Infof("volume %s is already umounted from global path %s", req.VolumeId, targetPath)
	}

	// detach after unmount from global
	if e := ns.ebsCli.Detach(ctx, req.GetVolumeId()); e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId:            ns.nodeID,
		MaxVolumesPerNode: ns.maxVolumesPerNode,
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				topologyZoneKey: ns.zone,
			},
		},
	}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	if e := resizeFS(req.GetVolumePath()); e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	klog.V(4).Infof("expanded volume %s, path: %s", req.GetVolumeId(), req.GetVolumePath())
	return &csi.NodeExpandVolumeResponse{}, nil
}

type findmntFS struct {
	Filesystems []struct {
		Target  string `json:"target"`
		Source  string `json:"source"`
		Fstype  string `json:"fstype"`
		Options string `json:"options"`
	} `json:"filesystems"`
}

func resizeFS(mountPoint string) error {
	out, e := exec.Command("findmnt", "-J", mountPoint).CombinedOutput()
	if e != nil {
		return e
	}
	var fs findmntFS
	if e := json.Unmarshal(out, &fs); e != nil {
		return e
	}
	if len(fs.Filesystems) != 1 {
		return fmt.Errorf("invalid mount source number: %d", len(fs.Filesystems))
	}

	if fs.Filesystems[0].Fstype != "ext4" {
		return fmt.Errorf("not supported fs type: %s", fs.Filesystems[0].Fstype)
	}

	if _, e = exec.Command("resize2fs", fs.Filesystems[0].Source).CombinedOutput(); e != nil {
		return e
	}
	return nil
}

func checkDevice(mountPoint string) error {
	out, e := exec.Command("findmnt", "-J", mountPoint).CombinedOutput()
	if e != nil {
		return e
	}
	var fs findmntFS
	if e := json.Unmarshal(out, &fs); e != nil {
		return e
	}
	if len(fs.Filesystems) != 1 {
		return fmt.Errorf("invalid mount source number: %d", len(fs.Filesystems))
	}
	if _, e := os.Stat(fs.Filesystems[0].Source); e != nil {
		return e
	}
	return nil
}
