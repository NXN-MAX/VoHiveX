FROM alpine:3.20
# Packages are installed inside this image only, never into the host system.
RUN apk add --no-cache ca-certificates tzdata setpriv libqmi python3 py3-yaml
WORKDIR /app
COPY release/vohive-dji-amd64 /app/vohive
COPY app/driver-lib.sh app/driver.sh app/single-start.sh /opt/vohivex/
RUN test "$(uname -m)" = x86_64 && chmod 755 /app/vohive
COPY app/scheduler/engine.py app/scheduler/server.py app/scheduler/managed_proxy.py app/scheduler/egress_ip.py app/scheduler/account.py app/scheduler/notifications.py app/scheduler/delivery.py /opt/vohivex/scheduler/
COPY app/proxy/vendor/mihomo-linux-amd64-compatible /opt/vohivex/proxy/mihomo
COPY app/proxy/vendor/LICENSE.mihomo app/proxy/vendor/LICENSE.jsQR /opt/vohivex/proxy/
COPY app/proxy/THIRD-PARTY.md /opt/vohivex/proxy/THIRD-PARTY.md
RUN chmod 755 /opt/vohivex/proxy/mihomo
COPY app/scheduler/assets /opt/vohivex/scheduler/assets
EXPOSE 7576
ENTRYPOINT ["/bin/sh", "/opt/vohivex/single-start.sh"]
