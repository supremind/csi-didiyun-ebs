package dataset

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	fuseUtil "bazil.org/fuse"
	"git.supremind.info/products/atom/proto/go/api"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/utils/exec"
	"supremind.com/ava/bolt-mount/pkg/builder"
	"supremind.com/ava/bolt-mount/pkg/fuse"
)

const (
	// params from storage class
	keyAPIEndpoint = "apiEndpoint"
	keyAPIUser     = "apiUser"
	keyDataSource  = "dataSource" // alluxio-fuse, global-volume
	// params from pvc
	keyDatasetName           = "atom.supremind.com/dataset-name"
	keyDatasetVersion        = "atom.supremind.com/dataset-version"
	keyDatasetCreator        = "atom.supremind.com/dataset-creator"
	keyDatasetPath           = "atom.supremind.com/dataset-path"
	keyDatasetIndexKey       = "atom.supremind.com/dataset-index-key"
	keyDatasetIndexURIRegexp = "atom.supremind.com/dataset-index-uri-regexp"

	driverNameCSI         = "kubernetes.io~csi"
	driverNameAlluxioFuse = "qiniu.com~alluxiofuse"

	datasetDBName         = "dataset.db"
	datasetIndexDirectory = "atom-dataset-index"

	csiTimeout = 10 * time.Second
)

type nodeServer struct {
	nodeID         string
	dsClient       dsClientInterface
	stagingFlights singleflight.Group
}

func NewNodeServer(nodeId string, timeout time.Duration) *nodeServer {
	return &nodeServer{
		nodeID:   nodeId,
		dsClient: newDsClient(timeout),
	}
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	if req.GetVolumeCapability().GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "Cap without mount access type")
	}

	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt { // aleady mounted
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// find dataset volume location
	var dataRoot string
	dataSource := req.GetVolumeContext()[keyDataSource]
	switch dataSource {
	case "alluxio-fuse":
		n := strings.Index(targetPath, driverNameCSI)
		if n == -1 {
			return nil, status.Errorf(codes.Internal, "unexpected target path %s", targetPath)
		}
		dataRoot = filepath.Join(targetPath[:n], driverNameAlluxioFuse)
	case "global-volume":
		dataRoot = "/global-volume"
	default:
		return nil, status.Error(codes.InvalidArgument, "empty data source")
	}

	dbPath := filepath.Join(req.GetStagingTargetPath(), datasetDBName)
	executor := exec.New()
	out, e := executor.Command("bolt-mount", "-d", "--dbpath", dbPath, "--rootpath", dataRoot, "--mountpoint", targetPath).CombinedOutput()
	if e != nil {
		errMsg := fmt.Sprintf("failed to mount dataset: %s: %s", e.Error(), string(out))
		glog.Errorf(errMsg)
		return nil, status.Error(codes.Internal, errMsg)
	}

	glog.V(4).Infof("dataset volume %s has been mounted.", req.GetVolumeId())
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	targetPath := req.GetTargetPath()
	if e := fuseUtil.Unmount(targetPath); e != nil {
		glog.Errorf("failed to unmount dataset: %s", e)
		return nil, status.Error(codes.Internal, e.Error())
	}

	glog.V(4).Infof("dataset volume %s has been unmounted.", req.GetVolumeId())
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetStagingTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capability missing in request")
	}

	result := ns.stagingFlights.DoChan(req.VolumeId, func() (interface{}, error) {
		if e := ns.doStage(context.TODO(), req); e != nil {
			return nil, e
		}
		return nil, nil
	})

	select {
	case ret := <-result:
		if ret.Err != nil {
			return nil, status.Error(codes.Internal, ret.Err.Error())
		}
	case <-time.After(csiTimeout):
		return nil, status.Errorf(codes.Unavailable, "volume %s is on the staging", req.GetVolumeId())
	}
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetStagingTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	// unstage will be called only if no pods mounted, it's safe to remove staged files
	if e := os.Remove(filepath.Join(req.GetStagingTargetPath(), datasetDBName)); e != nil {
		if !os.IsNotExist(e) {
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	if e := os.RemoveAll(filepath.Join(req.GetStagingTargetPath(), datasetIndexDirectory)); e != nil {
		if !os.IsNotExist(e) {
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	glog.V(4).Infof("dataset volume %s has been unstaged.", req.GetVolumeId())
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
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
		},
	}, nil
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeExpandVolume(context.Context, *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) doStage(ctx context.Context, req *csi.NodeStageVolumeRequest) (err error) {
	glog.V(4).Infof("start to stage dataset db/index for %s", req.GetVolumeId())

	params := req.GetVolumeContext()
	ep := params[keyAPIEndpoint]
	user := params[keyAPIUser]
	name := params[keyDatasetName]
	version := params[keyDatasetVersion]
	creator := params[keyDatasetCreator]
	annoKey := params[keyDatasetIndexKey]

	// check file existence first
	dbPath := filepath.Join(req.GetStagingTargetPath(), datasetDBName)
	indexFilePath := filepath.Join(req.GetStagingTargetPath(), datasetIndexDirectory, annoKey)
	_, dbExists := os.Stat(dbPath)
	if dbExists != nil && !os.IsNotExist(dbExists) {
		err = dbExists
		return
	}
	var indexExists error
	if annoKey != "" {
		_, indexExists = os.Stat(indexFilePath)
		if indexExists != nil && !os.IsNotExist(indexExists) {
			err = indexExists
			return
		}
	}
	if dbExists == nil && indexExists == nil {
		glog.V(4).Infof("found existing db/index file, reuse them")
		return
	}

	dsCli, e := ns.dsClient.datasetService(ep, user)
	if e != nil {
		err = e
		return
	}
	volCli, e := ns.dsClient.volumeService(ep, user)
	if e != nil {
		err = e
		return
	}
	stream, e := dsCli.ListDataItems(ctx, &api.ListDataItemsReq{DatasetVersion: &api.DatasetVersionRef{
		Dataset: name,
		Version: version,
		Creator: creator,
	}})
	if e != nil {
		err = e
		return
	}

	// use tmp path on flight
	tmpDBPath := filepath.Join(req.GetStagingTargetPath(), "db-"+uuid.NewUUID().String())
	tmpIndexPath := filepath.Join(req.GetStagingTargetPath(), "index-"+uuid.NewUUID().String())
	dbBuilder, e := builder.New(tmpDBPath)
	if e != nil {
		err = e
		return
	}
	defer func() {
		dbBuilder.Close()
		os.Remove(tmpDBPath)
		if err != nil {
			os.Remove(dbPath)
		}
	}()

	var indexBuilder *indexFileBuilder
	if annoKey != "" { // build index
		uriRegexp := params[keyDatasetIndexURIRegexp]
		var uriRe *regexp.Regexp
		if uriRegexp != "" {
			uriRe, e = regexp.Compile(uriRegexp)
			if e != nil {
				err = e
				return
			}
		}

		dsMountPath := params[keyDatasetPath]
		indexBuilder, e = newIndexFileBuilder(tmpIndexPath, dsMountPath, annoKey, uriRe)
		if e != nil {
			err = e
			return
		}
		defer func() {
			indexBuilder.Close()
			os.Remove(tmpIndexPath)
			if err != nil {
				os.Remove(indexFilePath)
			}
		}()
	}

	var volCache sync.Map
	fileItems := make(chan builder.FileItem, 100)
	dataItems := make(chan *api.DataItem, 100)
	errCh := make(chan error, 1)
	go func() {
		if indexBuilder != nil {
			// close dataItems to finish index builder that closes fileItems
			defer close(dataItems)
		} else {
			// close fileItems directly, dataItems won't be used
			defer close(fileItems)
		}

		for {
			item, e := stream.Recv()
			if e == io.EOF {
				break
			}
			if e != nil {
				errCh <- e
				return
			}
			for _, v := range item.GetMetas() {
				if v.GetVolumeRef().GetKind() != api.ResourceKindVolume {
					continue
				}
				volName := v.GetVolumeRef().GetName()
				volCreator := v.GetVolumeRef().GetCreator()
				vol, ok := volCache.Load(volCreator + "/" + volName)
				if !ok {
					volume, e := volCli.GetVolume(ctx, &api.GetVolumeReq{Name: volName, Creator: volCreator})
					if e != nil {
						errCh <- e
						return
					}
					volCache.Store(volCreator+"/"+volName, volume)
					vol = volume
				}

				vv := vol.(*api.Volume)
				fileItems <- builder.FileItem{
					Path: path.Join(vv.GetSpec().GetBucket(), vv.GetSpec().GetPath(), v.GetKey()),
					Meta: fuse.Meta{Size: v.GetContentLength()},
				}
				glog.V(6).Infof("added file item volume: %s/%s, key: %s, size: %d", volCreator, volName, v.GetKey(), v.GetContentLength())
			}

			if indexBuilder != nil {
				dataItems <- item
			}

			select {
			case <-ctx.Done():
				glog.V(3).Info("node stage request canceled")
				errCh <- ctx.Err()
				return
			default:
			}
		}

		errCh <- nil
	}()

	var eg errgroup.Group
	eg.Go(func() error {
		if e := dbBuilder.Build(fileItems, nil); e != nil {
			return e
		}

		// rename to db path in staging dir
		if e := os.Rename(tmpDBPath, dbPath); e != nil {
			return e
		}
		glog.V(5).Infof("built dataset db %s", dbPath)
		return nil
	})

	if indexBuilder != nil {
		eg.Go(func() error {
			defer close(fileItems)

			if e := indexBuilder.Build(dataItems, &volCache); e != nil {
				return e
			}

			// add index file entry after build
			fileItems <- builder.FileItem{
				Path: path.Join(datasetIndexDirectory, annoKey),
				Meta: fuse.Meta{Size: indexBuilder.Size(), RootPath: req.GetStagingTargetPath()},
			}

			// rename to index file path in staging dir
			if e := os.Mkdir(filepath.Join(req.GetStagingTargetPath(), datasetIndexDirectory), 0755); e != nil {
				if !os.IsExist(e) {
					return e
				}
			}
			if e := os.Rename(tmpIndexPath, indexFilePath); e != nil {
				return e
			}
			glog.V(5).Infof("built index file %s: count %d, size %d", indexFilePath, indexBuilder.Count(), indexBuilder.Size())
			return nil
		})
	}

	if e := eg.Wait(); e != nil {
		err = e
		return
	}

	if e := <-errCh; e != nil { // check input error
		err = fmt.Errorf("ListDataItems streaming error %s", e)
		return
	}

	glog.V(4).Infof("dataset db/index file for %s have been staged", req.GetVolumeId())
	return
}
