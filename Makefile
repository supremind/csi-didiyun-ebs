.PHONY: build-% container-% push-% clean test

REGISTRY_NAME = reg.supremind.info/products/ava/ava
BOLT_MOUNT_VERSION = 20190715-3859a5e

REV=$(shell date -u '+%Y%m%d')-$(shell git rev-parse --short HEAD)

IMAGE_NAME=$(REGISTRY_NAME)/$*

build-%:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./bin/$* ./cmd/$*

test:
	go test ./...

container-%: build-%
	docker build -t $*:latest -f ./cmd/$*/Dockerfile --label revision=$(REV) --build-arg BOLT_MOUNT_VERSION=$(BOLT_MOUNT_VERSION) .

push-%: container-%
	docker tag $*:latest $(IMAGE_NAME):$(REV)
	docker push $(IMAGE_NAME):$(REV)

clean:
	-rm -rf bin
