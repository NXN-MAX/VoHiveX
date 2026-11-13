# Build and Deployment

Run the following commands from the repository root. The Compose project name is `vohivex` (Compose requires lowercase), and the container name is `VoHiveX`. The target platforms are Linux AMD64, ARM64, and ARMv7, using `Dockerfile.vohivex` and `docker-compose.single.yml`.

## Requirements

- Frontend build machine: Python 3 and Node.js/npm. UPX is required only when regenerating binary patches.
- Runtime host: Docker Compose, plus `option` and either `qmi_wwan` (QMI mode) or `cdc_ether` (ECM mode), with their dependencies for the running kernel.
- Modem interfaces: the adapter registers a runtime serial match for `2ca3:4006`; MI_02 is used for AT. A vendor-specific MI_04 uses QMI, while the CDC ECM MI_04/MI_05 pair retains `cdc_ether`. It does not change the device USB ID or USB mode. Other devices continue to use the kernel's existing match table.
- Dependencies are installed only inside the container image; host system packages are not updated. Compose uses host networking so Network Debug can see the modem interfaces. `NET_ADMIN` lets the modem core configure an enabled cellular interface, whose default route uses metric `5000`; the NAS LAN route remains preferred and DNS is not changed. `NET_RAW` binds each diagnostic browser session to its selected cellular interface.

## Build

GitHub Actions builds `linux/amd64`, `linux/arm64`, and `linux/arm/v7` separately, runs startup tests, and publishes to `ghcr.io/nxn-max/vohivex`. Pulling `2.1.6` or `latest` selects the correct architecture automatically.

For the first source build, install Go 1.24+, Python 3, Node.js/npm, and UPX on the build machine, then run:

```sh
python3 app/prepare-build.py
python3 app/verify-build.py
docker build -f Dockerfile.vohivex -t vohivex:2.1.6 .
```

Use `--arch amd64`, `--arch arm64`, or `--arch armv7` to prepare a single architecture. The Vue 3 + Vite + Pinia + Antdv Next application in `web/` is the single frontend source. Each architecture uses its own version-pinned manifest and SHA-256 values; patch offsets are never reused with another upstream version. Rebuild a patch separately with `patch-release.py --arch <architecture> --original /path/to/original`.

## Configure and Start

1. Copy `.env.example` to `.env` and leave kernel detection automatic.
2. Run:

```sh
docker compose -f docker-compose.single.yml pull
docker compose -f docker-compose.single.yml up -d
```

The default web port is `7575`; the default username and password are both `admin`. On first start, VoHiveX creates `config/config.yaml`. Existing credentials, devices, and proxy settings are preserved. Legacy configurations automatically migrate `server.port` to `127.0.0.1:7576`, which is used only by the gateway inside the container. The external port and health check use `7575`.

Compose uses host networking and serves port `7575` on the Docker host. Protect LAN access with a strong VoHiveX password and the NAS firewall. After a host kernel upgrade, verify driver compatibility again.

## Runtime Checks

```sh
docker compose -f docker-compose.single.yml exec vohive /bin/sh /opt/vohivex/driver.sh inspect
docker compose -f docker-compose.single.yml exec vohive /bin/sh /opt/vohivex/driver.sh health
docker compose -f docker-compose.single.yml logs vohive
```

Health checks cover driver binding, device nodes, and the web service. Confirm the actual device state in the interface. Docker's restart policy recovers the container when a required process exits.

## ECM Discovery and Serial Port Conflicts

For DJI's factory-modified EC25-compatible `2ca3:4006` adapters with the QDC507 ECM layout, absence of `/dev/cdc-wdm*` is expected. The health check requires the AT node and ECM network interface instead. The bundled core recognizes this exact USB pair in its native AT fallback, including startup from saved IMEI-only configurations and reconnect discovery. The device keeps DJI's USB identity; the existing EC25 QMI driver reference remains available for QMI layouts. The pinned binary patch validates the entire original scanner function and architecture before changing its vendor guard. Existing Quectel/Qualcomm, QMI, and MBIM paths are preserved.

The gateway preserves complete native discovery responses. Its compatibility fallback for older separate cores supplements only this ECM layout and uses MI_02 for bounded, read-only `AT+CGSN` identity lookup when the authenticated request includes `with_imei=1`. It reads live worker bindings before considering a probe, avoids active workers' ports, and matches offline saved configurations by fresh IMEI rather than stale USB paths. Without `with_imei=1`, this fallback performs no serial query. It does not probe diagnostic or GNSS ports. An exclusive owner produces `discovery_error: serial_busy`; the response also includes `degraded: true` and `degraded_reason` for display. Inspect the Docker host's port owners (`sudo fuser -v /dev/ttyUSB*` or `sudo lsof /dev/ttyUSB*`); a container's PID namespace may hide the owning host process. A busy port alone does not identify which program owns it.

CI runs the actual packaged modem core against isolated sysfs and pseudo-terminal fixtures on AMD64, ARM64, and ARMv7. These cover the exact DJI ID, existing Quectel/Qualcomm ECM, QMI/MBIM discovery, and rejection of unrelated AT layouts. Run `python3 app/test-core-discovery.py --image <local-image> --platform linux/amd64` on a Linux Docker build host. No real USB devices or production data are mounted into these tests; physical modem operation must also be verified on the target host.

If host inspection confirms ModemManager owns this adapter and VoHiveX should manage it, the optional [79-mm-vohivex.rules](udev/79-mm-vohivex.rules) reserves only USB ID `2ca3:4006`. It applies to all adapters with that USB ID, so check for any other matching adapter first. Install it on the Docker host:

```sh
sudo install -m 0644 app/udev/79-mm-vohivex.rules /etc/udev/rules.d/79-mm-vohivex.rules
sudo udevadm control --reload-rules
```

Reloading rules does not release an already open port. Save ongoing modem work, then reconnect the affected adapter so it is rediscovered under the new policy, and rescan in Add Device. Do not stop ModemManager globally when it manages other hardware. VoHiveX does not install this rule or stop host services automatically. To undo the policy, remove only `/etc/udev/rules.d/79-mm-vohivex.rules`, reload rules, and reconnect the affected adapter again.

This rule follows ModemManager's documented [device-ignore tag](https://www.freedesktop.org/software/ModemManager/api/latest/ModemManager-Common-udev-tags.html) and [udev ordering and action rules](https://modemmanager.org/docs/modemmanager/port-and-device-detection/).

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

VoHiveX detects the host's running kernel automatically; no kernel version must be entered. Already loaded drivers are reused. Drivers needed by a connected adapter (`option` plus `qmi_wwan` or `cdc_ether`) are loaded from the mounted `/lib/modules/<running-kernel-version>`, including after hotplug. With no supported adapter connected, no driver load is required. Initialization reports an error and stops when a required driver cannot be loaded. It never installs driver packages or replaces the kernel. After upgrading the host kernel, ensure that the matching drivers are present on the host.
