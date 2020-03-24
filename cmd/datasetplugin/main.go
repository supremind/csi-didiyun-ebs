package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"time"

	"git.supremind.info/products/atom/csi-plugins/pkg/dataset"
)

func init() {
	flag.Set("logtostderr", "true")
}

var (
	endpoint    = flag.String("endpoint", "unix:///csi/csi.sock", "CSI endpoint")
	driverName  = flag.String("drivername", "dataset.csi.supremind.com", "name of the driver")
	nodeID      = flag.String("nodeid", "", "node id")
	timeout     = flag.Uint("timeout", 0, "rpc timeout, in second")
	showVersion = flag.Bool("version", false, "Show version.")
	// Set by the build process
	version = ""
)

func main() {
	flag.Parse()

	if *showVersion {
		baseName := path.Base(os.Args[0])
		fmt.Println(baseName, version)
		return
	}

	handle()
	os.Exit(0)
}

func handle() {
	driver, err := dataset.NewDriver(*driverName, *nodeID, *endpoint, time.Duration(*timeout)*time.Second, version)
	if err != nil {
		fmt.Printf("Failed to initialize driver: %s", err.Error())
		os.Exit(1)
	}
	driver.Run()
}
