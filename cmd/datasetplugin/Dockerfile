# syntax = 8tn49c6n.mirror.aliyuncs.com/docker/dockerfile:1.0-experimental
FROM reg.supremind.info/hub/atom/builder/golang-builder:latest AS builder

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN --mount=type=secret,id=netrc,dst=/root/.netrc go mod download

COPY . ./
RUN make build-datasetplugin

FROM 8tn49c6n.mirror.aliyuncs.com/library/ubuntu:18.04
RUN sed -i s/archive.ubuntu.com/mirrors.163.com/g /etc/apt/sources.list && \
    sed -i s/security.ubuntu.com/mirrors.163.com/g /etc/apt/sources.list && \
    apt update && apt install -y fuse
ARG BOLT_MOUNT_VERSION=dev
ADD http://pr6c01xnk.bkt.clouddn.com/release/bolt-mount-${BOLT_MOUNT_VERSION}.linux-amd64 /usr/local/bin/bolt-mount
ADD https://github.com/krallin/tini/releases/download/v0.18.0/tini /usr/local/bin/tini
RUN chmod +x /usr/local/bin/bolt-mount && chmod +x /usr/local/bin/tini
COPY --from=builder /workspace/bin/datasetplugin /datasetplugin
ENTRYPOINT ["tini", "--", "/datasetplugin"]
