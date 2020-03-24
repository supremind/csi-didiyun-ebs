module git.supremind.info/products/atom/csi-plugins

go 1.14

replace supremind.com/ava/bolt-mount => git.supremind.info/products/ava/bolt-mount.git v1.0.2-0.20190709082429-8b77ea3a1ed9

require (
	bazil.org/fuse v0.0.0-20180421153158-65cc252bf669
	git.supremind.info/products/atom/com v1.2.0
	git.supremind.info/products/atom/proto/go/api v1.0.11
	github.com/container-storage-interface/spec v1.1.0
	github.com/didiyun/didiyun-go-sdk v0.0.0-20190620073345-bca580aae22f
	github.com/gogo/protobuf v1.3.1
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubernetes-csi/csi-lib-utils v0.6.1
	github.com/kubernetes-csi/drivers v1.0.2
	github.com/pborman/uuid v1.2.0
	github.com/stretchr/testify v1.4.0
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a
	golang.org/x/oauth2 v0.0.0-20180821212333-d2e6202438be
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	google.golang.org/grpc v1.27.1
	k8s.io/apimachinery v0.0.0-20190602183612-63a6072eb563 // indirect
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.13.6
	k8s.io/utils v0.0.0-20190529001817-6999998975a7
	supremind.com/ava/bolt-mount v0.0.0
)
