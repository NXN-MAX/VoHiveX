## VoHiveX 2.1.5

Fix DJI `2ca3:4006` adapters that expose an ECM network interface (including the QDC507 layout) instead of QMI. Existing devices and persistent data remain compatible.

### Fixes

- Detect QMI and ECM USB layouts separately; preserve the kernel's `cdc_ether` binding and check the correct device nodes. ECM no longer requires `qmi_wwan` or a `cdc-wdm` node.
- Load only drivers needed by connected adapters, including hotplug after startup, and reuse project-owned dynamic IDs.
- Supplement authenticated device discovery for this ECM layout using MI_02 for AT and MI_04 for Ethernet. With identity discovery enabled, issue a bounded, read-only IMEI query without probing configured devices or diagnostic/GNSS ports.
- Keep unavailable adapters visible and show actionable driver/serial-busy reasons in the Add Device dialog. Provide an optional, USB-ID-scoped ModemManager ignore rule for hosts where it owns the ports.
- Add sysfs fixtures, authenticated API regression tests, and Linux pseudo-terminal tests for identity queries, exclusive locks, timeouts, and terminal-setting restoration.

No USB mode switch or host service change is performed automatically. The host's ModemManager policy remains an administrator choice; see [deployment troubleshooting](app/README.md#ecm-discovery-and-serial-port-conflicts).

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
