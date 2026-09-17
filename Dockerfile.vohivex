# Select the requested binaries without emulating the build helper stage.
FROM --platform=$BUILDPLATFORM alpine:3.20 AS binaries
ARG TARGETARCH
ARG TARGETVARIANT
COPY release/vohivex-* /inputs/release/
COPY app/proxy/vendor/mihomo-linux-* /inputs/proxy/
RUN case "$TARGETARCH/$TARGETVARIANT" in \
      amd64/) app_arch=amd64; proxy_arch=amd64-compatible ;; \
      arm64/|arm64/v8) app_arch=arm64; proxy_arch=arm64 ;; \
      arm/v7) app_arch=armv7; proxy_arch=armv7 ;; \
      *) echo "Unsupported platform: $TARGETARCH/$TARGETVARIANT" >&2; exit 1 ;; \
    esac && mkdir /out && \
    cp "/inputs/release/vohivex-$app_arch" /out/vohive && \
    cp "/inputs/proxy/mihomo-linux-$proxy_arch" /out/mihomo && \
    chmod 755 /out/vohive /out/mihomo

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS gateway
ARG TARGETARCH
ARG TARGETVARIANT
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/vohivex-gateway ./cmd/vohivex-gateway
RUN case "$TARGETARCH/$TARGETVARIANT" in \
      amd64/) goarm='' ;; \
      arm64/|arm64/v8) goarm='' ;; \
      arm/v7) goarm=7 ;; \
      *) echo "Unsupported gateway platform: $TARGETARCH/$TARGETVARIANT" >&2; exit 1 ;; \
    esac && CGO_ENABLED=0 GOOS=linux GOARCH="$TARGETARCH" GOARM="$goarm" \
    go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/vohivex-gateway ./cmd/vohivex-gateway

FROM alpine:3.20
LABEL org.opencontainers.image.source="https://github.com/NXN-MAX/VoHiveX" \
      org.opencontainers.image.title="VoHiveX" \
      org.opencontainers.image.version="2.1.0"
# Packages are installed inside this image only, never into the host system.
RUN apk add --no-cache ca-certificates tzdata setpriv libqmi
WORKDIR /app
COPY --from=binaries /out/vohive /app/vohive
COPY --from=binaries /out/mihomo /opt/vohivex/proxy/mihomo
COPY --from=gateway /out/vohivex-gateway /opt/vohivex/vohivex-gateway
COPY app/driver-lib.sh app/driver.sh app/single-start.sh /opt/vohivex/
COPY app/proxy/vendor/LICENSE.mihomo app/proxy/vendor/LICENSE.jsQR /opt/vohivex/proxy/
COPY app/proxy/THIRD-PARTY.md /opt/vohivex/proxy/THIRD-PARTY.md
COPY app/scheduler/assets /opt/vohivex/scheduler/assets
EXPOSE 7575
ENTRYPOINT ["/bin/sh", "/opt/vohivex/single-start.sh"]
