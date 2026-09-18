# VoHiveX Deployment

VoHiveX supports Linux AMD64, ARM64/AArch64, and ARMv7. Pulling `ghcr.io/nxn-max/vohivex:2.1.4` is recommended; Docker selects the matching architecture automatically. A standalone 386 Go gateway binary is also available, but it does not include a 386 modem core.

## Default Access

- Web port: `7575` (the container also listens on `7575`).
- Default username: `admin`.
- Default password: `admin`.

The default credentials are created only when no configuration exists. Existing credentials are not reset. Change the password after the first login.

## Pull and Start

1. The host must already provide `option`, `qmi_wwan`, and their dependencies for the running kernel, as well as Docker Compose.
2. Put `docker-compose.yml` and `.env.example` in the same directory, then copy `.env.example` to `.env`.
3. The kernel version is detected automatically. Drivers that are not already loaded are loaded from the matching directory for the running kernel. Initialization stops and records an error if that directory is missing or loading fails.
4. Set `VOHIVE_BIND_IP`. The default, `127.0.0.1`, permits local access only. For LAN access, use the host's LAN address.
5. Run:

```sh
docker compose config --quiet
docker compose pull
docker compose up -d
```

Open `http://<host-address>:7575`. Initialization creates the minimum required configuration automatically; credentials do not need to be prepared manually.

## Updates and Data

Before updating, stop the container and back up `.env`, `config`, `data`, `logs`, `driver-state`, and the deployment configuration. Updates do not overwrite user data. Legacy configurations only migrate the core service port to `127.0.0.1:7576`; credentials and all other settings are preserved. Update the web port mapping and health check in an older Compose file to use container port `7575`.

## Build from Source or a Deployment Package

A source build machine requires Go 1.24+, Python 3, Node.js/npm, and UPX. Run `python3 app/prepare-build.py`, then `docker build -t vohivex:2.1.4 .`. The complete deployment package already contains programs for all three architectures and the generated frontend, so no frontend dependencies are required on the deployment host.

Release packages include `SHA256SUMS`. After extraction, run `sha256sum -c SHA256SUMS` before use. `python3 app/package-single.py` creates `dist/VoHiveX-deploy.zip` and `dist/VoHiveX-single.zip`, the latter of which also includes the build source.

Compose pulls the published GHCR image by default. To deploy a locally built image, change the Compose `image` value to the local tag and set `pull_policy` to `never`.
