FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party ./third_party
# The cache mounts keep downloaded modules and compiled dependencies between builds. Without them every
# version recompiles go-chromecast, protobuf and grpc from scratch, which takes about two minutes on a Pi.
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /hub ./cmd/hub

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates shairport-sync avahi dbus
COPY --from=build /hub /hub
COPY run.sh /run.sh
CMD ["/run.sh"]
