# syntax = docker/dockerfile:1.0-experimental
FROM golang:1.15-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . ./
RUN go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./bin/ebsplugin ./cmd

FROM alpine:3.13
RUN apk add --no-cache util-linux e2fsprogs e2fsprogs-extra
COPY --from=builder /workspace/bin/ebsplugin /ebsplugin
ENTRYPOINT ["/ebsplugin"]
