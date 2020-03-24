
FROM alpine:3.9
RUN apk add --no-cache ca-certificates bash curl util-linux e2fsprogs
COPY ./bin/ebsplugin /ebsplugin
ENTRYPOINT ["/ebsplugin"]
