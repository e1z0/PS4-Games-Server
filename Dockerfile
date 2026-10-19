# SPDX-License-Identifier: GPL-2.0-only
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=0.1.0-dev
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /ps4-server ./cmd/ps4-server
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -g 10001 ps4 && adduser -D -u 10001 -G ps4 ps4 && mkdir /config /data /packages && chown ps4:ps4 /config /data
COPY --from=build /ps4-server /usr/local/bin/ps4-server
USER 10001:10001
WORKDIR /data
ENV PS4_DATA_DIR=/data PS4_LISTEN=:8080
EXPOSE 8080
VOLUME ["/config", "/data"]
HEALTHCHECK --interval=30s --timeout=6s --start-period=10s CMD ["/usr/local/bin/ps4-server", "health", "-config", "/config/config.json"]
ENTRYPOINT ["/usr/local/bin/ps4-server"]
CMD ["serve", "-config", "/config/config.json"]
