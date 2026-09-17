## VoHiveX 2.1.0

VoHiveX 2.1.0 replaces the Python management runtime with a statically linked Go gateway while preserving the existing web interface, configuration, scheduled tasks, imported SMS archives, proxy subscriptions, and modem core.

### Highlights

- Reimplemented the management gateway, scheduler, proxy manager, SMS archive, account settings, health checks, and metrics in Go.
- Added Linux binaries for amd64, arm64, aarch64, armv7, and 386, with SHA-256 checksums.
- Kept full Docker images for amd64, arm64/aarch64, and armv7.
- Added `/healthz` and Prometheus-compatible `/metrics` endpoints.
- Added database schema guards to prevent unsafe downgrades.
- Improved restart recovery: interrupted or ambiguous SMS sends are paused and never sent again automatically.
- Preserved SMS status value `0` during archive import and retained archive deduplication.
- Kept the built-in Mihomo proxy on the fixed SOCKS5 endpoint `127.0.0.1:17890`.
- Added IPv4 and IPv6 egress display support.
- Fixed the missing focus border on the SM-DP+ address field.
- Removed Python and PyYAML from the runtime image.
- Removed obsolete legacy installer files that used incompatible gateway arguments.

### Compatibility note

The 386 download is the Go gateway binary. The bundled legacy modem core has no upstream 386 build, so a 386 host must provide a compatible core service on `127.0.0.1:7576`. Docker images include the bundled modem core on amd64, arm64/aarch64, and armv7.
