# Agent Arena API: migrate then serve in one image.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
# Retried: the module proxy occasionally drops connections from a VM.
RUN --mount=type=cache,target=/go/pkg/mod \
    for i in 1 2 3 4 5; do go mod download && break || sleep 3; done && go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate \
 && for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do \
      CGO_ENABLED=0 GOOS=${target%/*} GOARCH=${target#*/} go build -trimpath \
        -o /out/connector/arena-${target%/*}-${target#*/} ./cmd/arena || exit 1; \
    done

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata git docker-cli && adduser -D -u 10001 arena
COPY --from=build /out/api /out/migrate /usr/local/bin/
COPY dev/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY fixtures/proofs /opt/arena/proofs
COPY --from=build /out/connector /opt/arena/connector
ENV ARENA_PROOFS_DIR=/opt/arena/proofs ARENA_CONNECTOR_DIR=/opt/arena/connector
# The arena-work volume is initialised from this directory, so it has to
# exist and belong to arena before the volume is first created.
RUN mkdir -p /var/lib/arena/work && chown arena:arena /var/lib/arena/work
USER arena
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
