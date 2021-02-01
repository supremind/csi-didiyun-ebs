.PHONY: build container push clean test
REV=$(shell date -u '+%Y%m%d')-$(shell git rev-parse --short HEAD)

build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./bin/ebsplugin ./cmd

test:
	go test ./...

clean:
	-rm -rf bin
