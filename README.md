![VoHiveX personal modem management and test platform](docs/images/vohivex-banner.png)

<p align="center">
  <a href="https://github.com/iniwex5/vohive">VoHive</a> ·
  <a href="https://go.dev/">Go</a> ·
  <a href="https://github.com/MetaCubeX/mihomo">Mihomo</a> ·
  <a href="https://github.com/vuejs/core">Vue 3</a> ·
  <a href="https://github.com/vitejs/vite">Vite</a> ·
  <a href="https://github.com/vuejs/pinia">Pinia</a> ·
  <a href="https://github.com/antdv-next/antdv-next">Antdv Next</a> ·
  <a href="https://github.com/Remix-Design/RemixIcon">Remix Icon</a>
</p>

# VoHiveX

**Original project:** [VoHive](https://github.com/iniwex5/vohive) by [iniwex5](https://github.com/iniwex5)<br>
**VoHiveX author:** [NXN-MAX](https://github.com/NXN-MAX) · **Version:** 2.1.4

VoHiveX is a personal modem management and testing platform derived from VoHive. It adds DJI 4G module compatibility, scheduled SMS, Mihomo subscription and node management, eSIM activation-code recognition, SMS archives, notifications, and a responsive Vue interface backed by a Go gateway.

## Highlights

### DJI 4G module compatibility

- Supports DJI first-generation 4G modules that expose USB ID `2ca3:4006`.
- Keeps the original DJI USB ID. Reflashing, rewriting the USB ID, and permanent host-driver modification are not required.
- Matches the device to the existing Linux `option` and `qmi_wwan` drivers at runtime, exposing AT serial and QMI interfaces to the container.
- Supports second-generation modules through compatible QMI or MBIM paths when provided by the module firmware and host kernel.

### Scheduled SMS

- Send once at a specified date and time or repeat by day, hour, minute, and second intervals.
- Create, edit, delete, start, pause, and inspect tasks, including the next run time and execution history.
- Newly created or edited tasks remain paused until explicitly started. Missed runs are skipped instead of being sent in a burst.
- Execution and delivery results can be forwarded through enabled Telegram, Feishu, QQ, Bark, Email, Pushplus, and Webhook channels.

### Subscription and node management

- Runs Mihomo inside the VoHiveX container and exposes a fixed built-in SOCKS5 endpoint at `127.0.0.1:17890`.
- Supports multiple HTTPS subscriptions, Clash YAML, and common proxy share links.
- Provides collapsible subscription lists, node selection, latency testing, VoWiFi country rules, and local outbound proxy instances.
- Proxy QR images are decoded in the browser and discarded immediately after recognition.

### eSIM activation-code recognition

- Recognizes activation codes from QR images, clipboard images, uploaded JPG/JPEG/PNG/WebP files, or a directly entered `LPA:1` link.
- Parses the SM-DP+ address, Matching ID, and optional confirmation code into the download form.
- Recognition only fills the form. No profile is written until the user reviews the values and presses **Start download**.
- Includes eUICC information, installed profiles, remarks, switching, and deletion where supported by the module and card.

### Device and message management

- Provides device discovery, radio status, AT and USSD terminals, SIM/eSIM information, card policies, VoWiFi status, and live logs.
- Includes a three-column SMS center with conversation search, unread state, delivery status, multi-select actions, and CSV/TXT/HTML/XML import and export.
- Offers a local Swagger UI at `/api/docs`, health checks at `/healthz`, and Prometheus-compatible metrics at `/metrics`.

## Supported architectures and modem paths

| Component | Supported targets |
| --- | --- |
| Full Docker runtime | Linux `amd64`, `arm64`/`aarch64`, and `armv7` |
| Standalone Go gateway | Linux `amd64`, `arm64`/`aarch64`, `armv7`, and `386` |
| Modem transport | AT serial with Qualcomm-compatible QMI or MBIM networking |
| Host kernel interfaces | `option`, `qmi_wwan`, USB serial, QMI control device, or a compatible MBIM path |
| DJI first generation | USB ID `2ca3:4006`, used without changing the USB ID |
| DJI second generation | Compatible QMI/MBIM layouts; available functions depend on firmware and USB composition |

The complete container is published only for architectures that have a compatible bundled modem core. The `386` download contains the Go gateway and must connect to a separately supplied compatible modem core.

## Docker installation

Images are published to:

- `maxnxxn/vohivex:2.1.4`
- `ghcr.io/nxn-max/vohivex:2.1.4`

Both repositories provide multi-architecture `2.1.4`, `v2.1.4`, and `latest` tags. Docker selects the correct image for the host automatically.

Defaults:

- Web port: `7575`
- Username: `admin`
- Password: `admin`

```sh
cp .env.example .env
docker compose pull
docker compose up -d
```

Open `http://<host>:7575`. Existing `config`, `data`, `logs`, and `driver-state` directories remain persistent during updates. Change the default password after the first login.

The container detects the running host kernel and reuses its existing modem drivers. It does not install kernel packages or replace the host kernel.

## Screenshots

Every screenshot below is generated from the local demo API. Device identifiers, IP addresses, phone numbers, messages, subscriptions, nodes, eSIM data, and tasks are fictional examples.

| Dashboard | Subscription and nodes |
| --- | --- |
| ![VoHiveX dashboard using fictional modem data](docs/images/dashboard.png) | ![VoHiveX proxy subscriptions using fictional nodes](docs/images/proxy.png) |

| eSIM QR or link recognition | SMS center |
| --- | --- |
| ![VoHiveX eSIM activation-code recognition using example data](docs/images/esim.png) | ![VoHiveX SMS center using fictional conversations](docs/images/sms.png) |

### Scheduled tasks

![VoHiveX scheduled tasks using fictional recipients and content](docs/images/tasks.png)

## Releases and documentation

- [GitHub Releases](https://github.com/NXN-MAX/VoHiveX/releases)
- [Docker Hub](https://hub.docker.com/r/maxnxxn/vohivex)
- [Driver and deployment notes](app/README.md)
- [Scheduled SMS notes](app/scheduler/README.md)
- [Mihomo and subscription notes](app/proxy/README.md)
- [Third-party notices](app/proxy/THIRD-PARTY.md)

## Responsible use and license

> [!CAUTION]
> **Commercial use is strictly prohibited. VoHiveX is intended only for personal research, learning, and testing on devices and phone numbers that you legally control.**

Do not use VoHiveX for verification-code collection, number rental, unsolicited or bulk messaging, fraud, unlawful proxy services, or any activity that violates local law or carrier terms.

The original project and third-party components retain their respective licenses. VoHiveX additions use the [Personal Non-Commercial License](LICENSE), which is not an OSI-approved open-source license.
