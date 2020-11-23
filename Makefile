.PHONY: build-% container-% push-% clean test
NETRC_PATH ?= $(HOME)/.netrc
IMAGE_NAME ?= reg.supremind.info/infra/didiyun/csi-ebs
BOLT_MOUNT_VERSION = 20200508-08723aa

REV=$(shell date -u '+%Y%m%d')-$(shell git rev-parse --short HEAD)

IMAGE_NAME=$(REGISTRY_NAME)/$*

build-%:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./bin/$* ./cmd/$*

test:
	go test ./...

docker-build:
	DOCKER_BUILDKIT=1 docker build \
	-t ebsplugin:latest \
	-f ./cmd/ebsplugin/Dockerfile \
	--network host \
	--secret id=netrc,src=$(NETRC_PATH) .

docker-push: docker-build
	docker tag ebsplugin:latest $(IMAGE_NAME):$(REV)
	docker push $(IMAGE_NAME):$(REV)

clean:
	-rm -rf bin
