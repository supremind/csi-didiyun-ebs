package dataset

import (
	"fmt"
	"time"

	"github.com/golang/glog"
)

type dsDriver struct {
	name     string
	nodeID   string
	version  string
	endpoint string
	timeout  time.Duration

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer
}

var (
	vendorVersion = "dev"
)

func NewDriver(driverName, nodeID, endpoint string, timeout time.Duration, version string) (*dsDriver, error) {
	if driverName == "" {
		return nil, fmt.Errorf("No driver name provided")
	}

	if nodeID == "" {
		return nil, fmt.Errorf("No node id provided")
	}

	if endpoint == "" {
		return nil, fmt.Errorf("No driver endpoint provided")
	}
	if version != "" {
		vendorVersion = version
	}

	glog.Infof("Driver: %v ", driverName)
	glog.Infof("Version: %s", vendorVersion)

	return &dsDriver{
		name:     driverName,
		version:  vendorVersion,
		nodeID:   nodeID,
		endpoint: endpoint,
		timeout:  timeout,
	}, nil
}

func (t *dsDriver) Run() {
	t.ids = NewIdentityServer(t.name, t.version)
	t.ns = NewNodeServer(t.nodeID, t.timeout)
	t.cs = NewControllerServer()

	s := NewNonBlockingGRPCServer()
	s.Start(t.endpoint, t.ids, t.cs, t.ns)
	s.Wait()
}
