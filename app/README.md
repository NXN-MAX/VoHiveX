# Build and Deployment

Run the following commands from the repository root. The Compose project name is `vohivex` (Compose requires lowercase), and the container name is `VoHiveX`. The target platforms are Linux AMD64, ARM64, and ARMv7, using `Dockerfile.vohivex` and `docker-compose.single.yml`.

## Requirements

- Frontend build machine: Python 3 and Node.js/npm. UPX is required only when regenerating binary patches.
- Runtime host: Docker Compose, plus `option`, `qmi_wwan`, and their dependencies for the running kernel.
- Modem interfaces: the adapter registers a runtime match for `2ca3:4006`; MI_02 is used for AT and MI_04 for QMI. It does not change the device USB ID. Other devices continue to use the kernel's existing match table.
- Dependencies are installed only inside the container image; host system packages are not updated. The current Compose configuration does not grant `NET_ADMIN` and therefore does not provide mobile-data dialing privileges.

## Build

GitHub Actions builds `linux/amd64`, `linux/arm64`, and `linux/arm/v7` separately, runs startup tests, and publishes to `ghcr.io/nxn-max/vohivex`. Pulling `2.1.4` or `latest` selects the correct architecture automatically.

For the first source build, install Go 1.24+, Python 3, Node.js/npm, and UPX on the build machine, then run:

```sh
python3 app/prepare-build.py
python3 app/verify-build.py
docker build -f Dockerfile.vohivex -t vohivex:2.1.4 .
```

Use `--arch amd64`, `--arch arm64`, or `--arch armv7` to prepare a single architecture. The Vue 3 + Vite + Pinia + Antdv Next application in `web/` is the single frontend source. Each architecture uses its own version-pinned manifest and SHA-256 values; patch offsets are never reused with another upstream version. Rebuild a patch separately with `patch-release.py --arch <architecture> --original /path/to/original`.

## Configure and Start

1. Copy `.env.example` to `.env`, set `VOHIVE_BIND_IP`, and leave kernel detection automatic.
2. Run:

```sh
docker compose -f docker-compose.single.yml pull
docker compose -f docker-compose.single.yml up -d
```

The default web port is `7575`; the default username and password are both `admin`. On first start, VoHiveX creates `config/config.yaml`. Existing credentials, devices, and proxy settings are preserved. Legacy configurations automatically migrate `server.port` to `127.0.0.1:7576`, which is used only by the gateway inside the container. The external port and health check use `7575`.

The default binding is host address `127.0.0.1`. For LAN access, set the host's LAN address in `.env`. After a host kernel upgrade, verify driver compatibility again.

## Runtime Checks

```sh
docker compose -f docker-compose.single.yml exec vohive /bin/sh /opt/vohivex/driver.sh inspect
docker compose -f docker-compose.single.yml exec vohive /bin/sh /opt/vohivex/driver.sh health
docker compose -f docker-compose.single.yml logs vohive
```

Health checks cover driver binding, device nodes, and the web service. Confirm the actual device state in the interface. Docker's restart policy recovers the container when a required process exits.

## Updates and Backups

Before updating, stop the container and back up `.env`, `config`, `data`, `logs`, `driver-state`, and the active deployment configuration. Rebuild after replacing program files; do not overwrite persistent directories.

```sh
python3 app/package-single.py
```

Packages are written to `dist/`. Use `--output-dir /path/to/output` to choose another directory:

- `VoHiveX-deploy.zip`: generated frontend, Linux programs for all three architectures, Mihomo, and Docker build inputs. It can build the image directly.
- `VoHiveX-single.zip`: everything in the deployment package plus frontend build scripts, fonts, icons, documentation, and screenshots. It can rebuild the frontend and package the project again.

Both packages retain `Dockerfile`, `Dockerfile.vohivex`, `docker-compose.yml`, and `docker-compose.single.yml`, and include `SHA256SUMS`. They exclude user configuration, SMS data, logs, subscriptions, dependency caches, and development archives. The extracted project directory may be renamed.

```sh
python3 app/verify-build.py
```

This command validates local image inputs and binary checksums; it does not replace an actual Docker build. Generated binaries and frontend assets are not stored in the Git source. On a machine with Python 3, Node.js/npm, and UPX, run `python3 app/prepare-build.py` to download version-pinned upstream files, verify SHA-256 values, generate customized binaries and the frontend, and prepare Mihomo. The script does not install system packages. GitHub Actions performs these steps in an isolated CI environment, runs per-architecture startup tests, and publishes to GitHub Container Registry. Pull requests do not publish images.

## Stop and Undo Driver Registration

```sh
docker compose -f docker-compose.single.yml stop vohive
docker compose -f docker-compose.single.yml run --rm --no-deps --entrypoint /bin/sh vohive /opt/vohivex/driver.sh cleanup
docker compose -f docker-compose.single.yml down
```

Stopping the container does not immediately unbind the device. `cleanup` removes only matches and bindings registered by this project and attempts to unload modules that it loaded. A module in use by another device is never force-unloaded. If `option1` does not support `remove_id`, the dynamic match is cleared only after the module is safely unloaded or the host restarts. Keep `driver-state` until cleanup finishes.

## Module Documentation

- [Scheduled SMS](scheduler/README.md)
- [Proxy management](proxy/README.md)
- [API documentation](docs/README.md)
- [Brand assets](branding/README.md)
- [Third-party components and licenses](proxy/THIRD-PARTY.md)

## Automatic Kernel Detection

At container start, VoHiveX detects the host's running kernel automatically; no kernel version must be entered. Already loaded drivers are reused. Otherwise, `option`, `qmi_wwan`, and their dependencies are loaded from the mounted `/lib/modules/<running-kernel-version>`. Initialization reports an error and stops when that directory is missing or driver loading fails. It never installs driver packages or replaces the kernel. After upgrading the host kernel, ensure that the matching drivers are present on the host.
