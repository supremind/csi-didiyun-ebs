.PHONY: build-% container-% push-% clean test
NETRC_PATH ?= $(HOME)/.netrc
REGISTRY_NAME ?= reg.supremind.info/products/atom/csi-plugins
BOLT_MOUNT_VERSION = 20190715-3859a5e

REV=$(shell date -u '+%Y%m%d')-$(shell git rev-parse --short HEAD)

IMAGE_NAME=$(REGISTRY_NAME)/$*

build-%:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./bin/$* ./cmd/$*

test:
	go test ./...

docker-build-%:
	DOCKER_BUILDKIT=1 docker build \
	-t $*:latest \
	-f ./cmd/$*/Dockerfile \
	--network host \
	--secret id=netrc,src=$(NETRC_PATH) \
	--build-arg BOLT_MOUNT_VERSION=$(BOLT_MOUNT_VERSION) .

docker-push-%: docker-build-%
	docker tag $*:latest $(IMAGE_NAME):$(REV)
	docker push $(IMAGE_NAME):$(REV)

clean:
	-rm -rf bin
