FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party ./third_party
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /hub ./cmd/hub

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates shairport-sync avahi dbus
COPY --from=build /hub /hub
COPY run.sh /run.sh
CMD ["/run.sh"]
