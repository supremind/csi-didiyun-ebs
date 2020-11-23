# syntax = docker/dockerfile:1.0-experimental
FROM reg.supremind.info/hub/ci/golang-builder:latest AS builder

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN --mount=type=secret,id=netrc,dst=/root/.netrc go mod download

COPY . ./
RUN make build

FROM reg.supremind.info/hub/ci/alpine-base:latest
RUN apk add --no-cache util-linux e2fsprogs e2fsprogs-extra
COPY --from=builder /workspace/bin/ebsplugin /ebsplugin
ENTRYPOINT ["/ebsplugin"]
