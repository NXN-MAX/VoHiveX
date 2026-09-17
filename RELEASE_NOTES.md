## VoHiveX 2.1.2

VoHiveX 2.1.2 fixes live log rendering and SMS conversation refresh across all devices. Existing configuration, devices, messages, scheduled tasks, proxy subscriptions, and account credentials remain compatible.

### Fixes

- Render structured live-log records as readable timestamp, level, device, and message fields instead of `[object Object]`.
- Keep compatibility with plain-text log records and multiple API response shapes.
- Expand the SMS `all devices` request into per-device upstream requests so the legacy modem core receives the required device and IMSI values.
- Merge conversations with device-aware keys to avoid collisions when different devices use the same contact number.
- Continue loading available conversations when one device is temporarily offline.

### Compatibility

- No persistent-data or configuration migration is required from 2.1.1.
- Full Docker images are published for amd64, arm64/aarch64, and armv7.
- Standalone Go gateway binaries are published for amd64, arm64/aarch64, armv7, and 386 with `SHA256SUMS`.
