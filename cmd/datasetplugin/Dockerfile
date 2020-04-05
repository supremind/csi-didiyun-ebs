# syntax = 8tn49c6n.mirror.aliyuncs.com/docker/dockerfile:1.0-experimental
FROM reg.supremind.info/hub/atom/builder/golang-builder:latest AS builder

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN --mount=type=secret,id=netrc,dst=/root/.netrc go mod download

COPY . ./
RUN make build-datasetplugin

FROM reg.supremind.info/hub/atom/builder/alpine-base:latest
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apk/repositories && \
    apk update && apk add --no-cache fuse
ARG BOLT_MOUNT_VERSION=dev
ADD http://pr6c01xnk.bkt.clouddn.com/release/bolt-mount-${BOLT_MOUNT_VERSION}.linux-amd64 /usr/local/bin/bolt-mount
ADD https://github.com/krallin/tini/releases/download/v0.18.0/tini /usr/local/bin/tini
RUN chmod +x /usr/local/bin/bolt-mount && chmod +x /usr/local/bin/tini
COPY --from=builder /workspace/bin/datasetplugin /datasetplugin
ENTRYPOINT ["tini", "--", "/datasetplugin"]
