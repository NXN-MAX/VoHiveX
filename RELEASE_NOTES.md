## VoHiveX 2.1.4

VoHiveX 2.1.4 restores device and SIM details used by the original modem API, improves live-log stability, and prevents private cellular addresses from appearing on dashboard cards. Existing configuration, devices, messages, scheduled tasks, proxy subscriptions, and account credentials remain compatible.

### Fixes

- Restore device status fields, cellular radio details, data-channel state, phone number, active eSIM information, and carrier display through compatibility-aware API handling.
- Restore eUICC manufacturer, available-space, profile remark, and profile-management data when supplied by the modem core.
- Keep live logs stable during automatic refresh and render structured entries with compact timestamp, level, device, and message fields.
- Correct SMS conversation metadata, read-state handling, message direction, delivery status, and periodic refresh behavior.
- Expand the bundled API documentation to cover legacy card-policy, continued USSD, notification-test, scheduled-task, managed-proxy, SMS archive, account, and system-information endpoints.
- Hide private cellular interface addresses from dashboard cards. VoWiFi cards continue to show the applicable proxy egress address.
- Restore missing device-creation fields and improve compatibility with legacy request and response shapes.

### Compatibility

- No persistent-data or configuration migration is required from 2.1.3.
- Full Docker images are published for amd64, arm64/aarch64, and armv7.
- Standalone Go gateway binaries are published for amd64, arm64/aarch64, armv7, and 386 with `SHA256SUMS`.
