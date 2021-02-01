package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/supremind/csi-didiyun-ebs/pkg/didiyun/ebs"
	"k8s.io/klog"
)

var (
	endpoint = flag.String("endpoint", "unix:///csi/csi.sock", "CSI endpoint")
	nodeID   = flag.String("nodeid", "", "node id")
	regionID = flag.String("regionid", "", "region id")
	zoneID   = flag.String("zoneid", "", "zone id")
	token    = flag.String("token", "", "ebs api token")
	timeout  = flag.Uint("timeout", 30, "ebs rpc timeout, in second")
)

func main() {
	flag.Set("alsologtostderr", "true")
	flag.Parse()
	syncKlog()

	cfg := &ebs.DriverConfig{
		NodeID:   *nodeID,
		RegionID: *regionID,
		ZoneID:   *zoneID,
		Token:    *token,
		Endpoint: *endpoint,
		Timeout:  time.Duration(*timeout) * time.Second,
	}
	driver, e := ebs.NewDriver(cfg)
	if e != nil {
		fmt.Printf("Failed to initialize driver: %s", e)
		os.Exit(1)
	}
	driver.Run()
}

func syncKlog() {
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)
	// Sync the glog and klog flags.
	flag.CommandLine.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			f2.Value.Set(value)
		}
	})
}
