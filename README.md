![VoHiveX personal modem management and test platform](docs/images/vohivex-banner.png)

# VoHiveX

**Original author:** [iniwex5](https://github.com/iniwex5) · Original project: [VoHive](https://github.com/iniwex5/vohive)

**VoHiveX author:** [NXN-MAX](https://github.com/NXN-MAX) · **Version: 2.1.1**

VoHiveX extends VoHive with DJI first and second generation 4G modem compatibility, scheduled SMS, a built-in Mihomo proxy manager, SMS archive tools, a redesigned responsive interface, and a Go management gateway. The existing modem core, configuration, and persistent data formats remain compatible.

## Acceptable use

> [!CAUTION]
> **Commercial use is strictly prohibited. VoHiveX is for personal research, learning, and testing on devices that you own.**
>
> Use only personal phone numbers that you legally control. Do not use VoHiveX for verification-code collection, number rental, unsolicited or bulk messaging, fraud, unlawful proxy services, or any activity that violates local law or carrier terms.

The original project and third-party components keep their respective licenses. VoHiveX additions use the [Personal Non-Commercial License](LICENSE), which is not an OSI-approved open-source license.

## Main features

### DJI modem compatibility

- First generation DJI 4G modules using USB ID `2ca3:4006` are matched at runtime without reflashing or changing the USB ID.
- The existing host `option` and `qmi_wwan` drivers provide the AT serial and QMI interfaces.
- Second generation devices use the standard QMI or MBIM detection path. Hardware and firmware combinations not tested by this project may still require an additional mapping.
- Driver adaptation runs inside the VoHiveX container. It does not replace the host kernel or install a host driver package.

### Scheduled SMS

- Send once at a chosen date and time, or repeat at a day, hour, minute, and second interval.
- Create, edit, delete, start, and pause tasks, and review execution history and the next run time.
- Notify enabled Telegram, Feishu, QQ, Bark, Email, Pushplus, and Webhook channels after execution and after a delivery result becomes available.
- Missed tasks are skipped after downtime. Failed or ambiguous sends are paused and are never retried automatically.

### Mihomo proxy management

- Manage subscriptions and nodes, VoWiFi roaming proxies, and local outbound proxies from one module.
- Import multiple HTTPS subscriptions, Clash YAML, and common share links; select nodes and test HTTPS latency.
- QR images are decoded in the browser and are discarded without upload or caching.
- The fixed `Subscription node · Built-in proxy` entry uses SOCKS5 at `127.0.0.1:17890`.
- Disabling a proxy makes associated country rules fall back to direct access.

### Interface and administration

- Responsive light and black dark themes, Remix Icon controls, SIM-card device tiles, and IPv4/IPv6 display.
- Conversation-based SMS center with import, export, selection, bulk state changes, and archive deduplication.
- eSIM activation-code parsing from pasted text, clipboard images, or JPG, JPEG, PNG, and WebP files. Profile installation starts only after the user presses the download button.
- Username and password settings, local Swagger UI at `/api/docs`, system/build/driver/proxy information, `/healthz`, and Prometheus-compatible `/metrics`.

## Go runtime

The management gateway, reverse proxy, scheduler, SMS archive, account settings, Mihomo lifecycle, health checks, and metrics are implemented in Go. The container no longer installs Python or PyYAML. Existing `config.yaml`, scheduler SQLite data, imported SMS archives, proxy subscriptions, and web assets are reused during an upgrade.

The bundled legacy modem core was already a Go executable but its upstream source is not present in this repository. For that reason, the full container remains available on the three architectures for which a compatible modem core exists.

| Deliverable | amd64 | arm64 | aarch64 | armv7 | 386 |
| --- | --- | --- | --- | --- | --- |
| Full Docker image | Yes | Yes | Alias of arm64 | Yes | No |
| Go gateway binary | Yes | Yes | Alias of arm64 | Yes | Yes |

The 386 gateway must connect to a separately supplied compatible modem core on `127.0.0.1:7576`. This repository does not claim 386 modem-core support.

## Docker installation

Images:

- `ghcr.io/nxn-max/vohivex:2.1.1`
- `maxnxxn/vohivex:2.1.1`

The multi-platform tag selects the host architecture automatically. Architecture tags are `2.1.1-amd64`, `2.1.1-arm64`, `2.1.1-aarch64`, and `2.1.1-armv7`.

Defaults:

- Web port: `7575`
- Username: `admin`
- Password: `admin`

Default credentials are created only when no configuration exists. Upgrades keep the current username, password, devices, messages, tasks, and proxy data. Change the default password after the first login.

1. Download `docker-compose.yml` and `.env.example` into one directory.
2. Copy `.env.example` to `.env`. No kernel version setting is required.
3. Set `VOHIVE_BIND_IP` to the NAS LAN address when other LAN devices need access. The example defaults to `127.0.0.1`.
4. Start the service:

```sh
docker compose pull
docker compose up -d
```

Open `http://<host>:7575`. Persistent state is stored in `config`, `data`, `logs`, and `driver-state`. Back up those directories, `.env`, and the Compose file before an update.

The container detects the running host kernel automatically. It reuses loaded modules or loads `option`, `qmi_wwan`, and their dependencies from `/lib/modules/<running-kernel>`. Missing modules stop driver initialization with an error; the container does not install a kernel package or replace the host kernel.

## Binary downloads

GitHub Releases provides the following statically linked Linux gateway binaries plus `SHA256SUMS`:

- `vohivex-amd64`
- `vohivex-arm64`
- `vohivex-aarch64`
- `vohivex-armv7`
- `vohivex-386`

Verify and install a binary, using the file for your architecture:

```sh
sha256sum -c SHA256SUMS --ignore-missing
chmod 0755 vohivex-amd64
./vohivex-amd64 -version
install -m 0755 vohivex-amd64 /usr/local/bin/vohivex-gateway
```

The standalone gateway expects:

- a compatible VoHive modem core at `127.0.0.1:7576`;
- a writable configuration file and data directory;
- the built frontend assets;
- a Mihomo binary when managed proxy features are used.

Example startup:

```sh
export CONFIG_PATH=/etc/vohivex/config.yaml
export VOHIVEX_DATA=/var/lib/vohivex
export SCHEDULER_ASSETS=/opt/vohivex/assets
export MIHOMO_BINARY=/opt/vohivex/mihomo

vohivex-gateway -prepare-config
vohivex-gateway
```

The gateway listens on `0.0.0.0:7575` by default and keeps the modem core on the loopback-only port `7576`. Set `SCHEDULER_PORT`, `VOHIVE_UPSTREAM_HOST`, or `VOHIVE_UPSTREAM_PORT` only when the surrounding service layout differs.

### Binary update

1. Back up `config.yaml` and the complete data directory.
2. Download the new binary and `SHA256SUMS` from the same release.
3. Verify the checksum and version.
4. Stop the current gateway.
5. Atomically replace the executable and restart the service.
6. Confirm `/healthz`, the web login, the device state, scheduled tasks, SMS history, and the managed proxy state.

Do not downgrade after a release creates a newer database schema. The gateway rejects a database that is newer than the schema it understands instead of silently damaging it.

## Frontend development

The management interface is a standard Vue 3 single-page application in `web/`, built with Vite, Pinia, Vue Router, TypeScript, and Antdv Next. Element Plus is no longer part of the runtime frontend.

```sh
npm ci --prefix web
npm run typecheck --prefix web
npm run dev --prefix web
```

The development server listens on `http://127.0.0.1:18765` and proxies `/api` to `http://127.0.0.1:7575`. A production asset bundle is assembled with `python3 app/scheduler/build-assets.py`.

## Build and release

Build the gateway locally with Go 1.24 or newer:

```sh
go test ./...
CGO_ENABLED=0 go build -trimpath -o vohivex-gateway ./cmd/vohivex-gateway
```

Build a local Docker image after preparing the pinned modem-core and Mihomo inputs:

```sh
python3 app/prepare-build.py
docker build -t vohivex:2.1.1 .
```

GitHub Actions tests the Go code, cross-compiles five gateway downloads, builds and smoke-tests the amd64, arm64, and armv7 images, publishes the multi-platform GHCR and Docker Hub tags, generates SHA-256 checksums, and creates a GitHub Release for a matching `v2.1.1` tag.

Additional documentation:

- [Driver and deployment notes](app/README.md)
- [Scheduled SMS notes](app/scheduler/README.md)
- [Mihomo and subscription notes](app/proxy/README.md)
- [Third-party notices](app/proxy/THIRD-PARTY.md)
- [Brand font license](app/branding/README.md)

## Screenshots

All screenshots use fictional devices, nodes, phone numbers, messages, and tasks. They contain no real subscription URLs, credentials, or personal communications.

### Dashboard

![VoHiveX dashboard with a fictional SIM device](docs/images/dashboard.png)

### Proxy management

![VoHiveX proxy management with fictional nodes](docs/images/proxy.png)

### SMS center

![VoHiveX SMS center with a fictional conversation](docs/images/sms.png)

### Scheduled tasks

![VoHiveX scheduled tasks with a paused fictional task](docs/images/tasks.png)
