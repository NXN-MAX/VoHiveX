# VoHiveX

Original author: [iniwex5](https://github.com/iniwex5) · VoHiveX author: [NXN-MAX](https://github.com/NXN-MAX)

VoHiveX is a personal modem management and testing platform with device monitoring, SMS, scheduled messages, notifications, eSIM management, AT and USSD terminals, VoWiFi, and built-in Mihomo proxy management. It supports QMI modules that retain the original `2ca3:4006` USB ID, without rewriting the ID. Feature availability depends on the device, drivers, and carrier.

## Default Access

| Item | Default |
| --- | --- |
| Web port | `7575` |
| Username | `admin` |
| Password | `admin` |

Default credentials are created only for a new installation. Updates preserve the existing configuration and password. Change the password after the first login.

## Images and Architectures

- Docker Hub: `maxnxxn/vohivex:2.1.4`
- GHCR: `ghcr.io/nxn-max/vohivex:2.1.4`
- Multi-architecture tags: `2.1.4`, `v2.1.4`, and `latest`, with automatic selection for AMD64, ARM64, and ARMv7.
- Architecture-specific tags: `2.1.4-amd64`, `2.1.4-arm64`, `2.1.4-aarch64`, and `2.1.4-armv7`.

```sh
docker pull maxnxxn/vohivex:2.1.4
```

Download `docker-compose.yml` and `.env.example` from the [GitHub repository](https://github.com/NXN-MAX/VoHiveX). Change the Compose `image` value to `maxnxxn/vohivex:2.1.4`, copy `.env.example` to `.env`, and set `VOHIVE_BIND_IP`. The kernel version is detected automatically and must not be entered manually. Then run `docker compose pull && docker compose up -d`.

Follow the [deployment guide](https://github.com/NXN-MAX/VoHiveX/blob/main/DEPLOY.md) to mount the configuration, data, and devices. Preserve `config`, `data`, `logs`, and `driver-state`; never bake user data into the image. ARM images are validated through emulated startup tests, while USB modem functions still require testing on physical hardware.

## Automated GitHub Builds and Publishing

GitHub Actions builds all three architectures on changes to `main`, matching version tags, or manual runs. After all startup tests pass, it publishes to GHCR and synchronizes images from the same commit to Docker Hub. Pull requests are tested without publishing.

Configure these values once under GitHub repository **Settings → Secrets and variables → Actions**:

- Variable: `DOCKERHUB_USERNAME` = `maxnxxn`.
- Secret: `DOCKERHUB_TOKEN` = a Docker Hub Read & Write access token. Delete permission is not required.

Docker Hub publishing is skipped when the username variable is absent. When publishing is enabled, a missing or expired token causes the Docker Hub job to fail clearly. The token remains in GitHub encrypted secrets and is never written to the source tree or image. The paid Docker Hub automated-build service and separate Docker Hub access to the GitHub source are not required.

## Usage Restrictions

Use VoHiveX only for internal testing with personally owned devices and phone numbers that you legally control. Commercial use, third-party numbers, and unlawful activity are strictly prohibited. See the [project license](https://github.com/NXN-MAX/VoHiveX/blob/main/LICENSE) for the full terms and third-party rights.
