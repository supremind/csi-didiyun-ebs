package ebs

import (
	"errors"

	didiyunClient "git.supremind.info/products/atom/didiyun-client/pkg"
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	keyRegion     = "regionID"
	keyZone       = "zoneID"
	keyType       = "type"
	keyDeviceName = "deviceName"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	ebsCli didiyunClient.EbsClient
}

func NewControllerServer(d *csicommon.CSIDriver, cli didiyunClient.EbsClient) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		ebsCli:                  cli,
	}
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume Name cannot be empty")
	}
	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}

	params := req.GetParameters()
	region := params[keyRegion]
	zone := params[keyZone]
	typ := params[keyType]
	size := (req.GetCapacityRange().GetRequiredBytes() + (1 << 30) - 1) / (1 << 30)

	resID, e := cs.ebsCli.Create(ctx, region, zone, req.GetName(), typ, size)
	if e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}

	createVolumeResponse := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      resID, // ebs uuid as volume id
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext: req.GetParameters(),
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						topologyZoneKey: zone,
					},
				},
			},
		},
	}
	klog.V(4).Infof("volume created: %s for %s, %v", resID, req.GetName(), req.GetParameters())
	return createVolumeResponse, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if e := cs.ebsCli.Delete(ctx, req.GetVolumeId()); e != nil {
		if errors.Is(e, didiyunClient.NotFound) {
			klog.V(3).Infof("couldn't delete not found volume %s", req.GetVolumeId())
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, e.Error())
	}

	klog.V(4).Infof("volume deleted: %s", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	caps := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	}

	var csc []*csi.ControllerServiceCapability
	for _, c := range caps {
		csc = append(csc, csicommon.NewControllerServiceCapability(c))
	}
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: csc,
	}, nil
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	for _, cap := range req.VolumeCapabilities {
		if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: ""}, nil
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.GetVolumeContext(),
			VolumeCapabilities: req.GetVolumeCapabilities(),
			Parameters:         req.GetParameters(),
		},
	}, nil
}

func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
