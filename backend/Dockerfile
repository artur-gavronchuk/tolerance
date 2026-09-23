# Agent Arena API: migrate + seed + serve in one image.
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
 && CGO_ENABLED=0 go build -trimpath -o /out/seed ./cmd/seed

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 arena
COPY --from=build /out/api /out/migrate /out/seed /usr/local/bin/
COPY dev/entrypoint.sh /usr/local/bin/entrypoint.sh
USER arena
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
