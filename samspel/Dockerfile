FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /hub ./cmd/hub

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=build /hub /hub
CMD ["/hub", "-port", "8080", "-state", "/data/groups.json"]
