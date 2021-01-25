package ebs

import (
	"errors"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	didiyunClient "github.com/supremind/didiyun-client/pkg"
	"k8s.io/klog"
)

const (
	driverName      = "didiyun-ebs.csi.supremind.com"
	csiVersion      = "1.0.0"
	topologyZoneKey = "topology." + driverName + "/zone"
)

type ebs struct {
	endpoint         string
	idServer         csi.IdentityServer
	nodeServer       csi.NodeServer
	controllerServer csi.ControllerServer
}

type DriverConfig struct {
	NodeID   string
	RegionID string
	ZoneID   string
	Endpoint string
	Token    string
	Timeout  time.Duration
}

func NewDriver(cfg *DriverConfig) (*ebs, error) {
	cli, e := didiyunClient.New(&didiyunClient.Config{Token: cfg.Token, Timeout: cfg.Timeout})
	if e != nil {
		return nil, e
	}

	driver := csicommon.NewCSIDriver(driverName, csiVersion, cfg.NodeID)
	if driver == nil {
		return nil, errors.New("failed to create csi common driver")
	}
	driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})
	return &ebs{
		idServer:         NewIdentityServer(driver),
		nodeServer:       NewNodeServer(driver, cfg.NodeID, cfg.ZoneID, cli.Ebs()),
		controllerServer: NewControllerServer(driver, cli.Ebs()),
		endpoint:         cfg.Endpoint,
	}, nil
}

func (t *ebs) Run() {
	klog.Infof("Starting csi-plugin Driver: %v version: %v", driverName, csiVersion)
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(t.endpoint, t.idServer, t.controllerServer, t.nodeServer)
	s.Wait()
}
