.PHONY: build container push clean test
NETRC_PATH ?= $(HOME)/.netrc
DOCKER_REG ?= reg.supremind.info/infra/didiyun/csi-ebs

REV=$(shell date -u '+%Y%m%d')-$(shell git rev-parse --short HEAD)

build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./bin/ebsplugin ./cmd/ebsplugin

test:
	go test ./...

docker-build:
	DOCKER_BUILDKIT=1 docker build \
	-t ebsplugin:latest \
	-f ./cmd/ebsplugin/Dockerfile \
	--network host \
	--secret id=netrc,src=$(NETRC_PATH) .

docker-push: docker-build
	docker tag ebsplugin:latest $(DOCKER_REG):$(REV)
	docker push $(DOCKER_REG):$(REV)

clean:
	-rm -rf bin
