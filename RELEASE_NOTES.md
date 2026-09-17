## VoHiveX 2.1.1

VoHiveX 2.1.1 replaces the legacy patched frontend bundle with a maintainable Vue 3 application and restores the management features in a responsive interface. Existing configuration, devices, messages, scheduled tasks, proxy subscriptions, and account credentials remain compatible.

### Highlights

- Rebuilt the management interface with Vue 3, Vite, Pinia, Vue Router, TypeScript, and Antdv Next.
- Removed the obsolete minified-asset patch pipeline and now builds the runtime frontend directly from `web/`.
- Restored device overview, traffic analysis, eSIM, AT, USSD, card policy, and device configuration views.
- Restored subscription and node management, roaming country rules, local outbound proxies, connection state, and IPv4/IPv6 egress display.
- Restored the three-column SMS interface with delivery state, conversation selection, bulk read/unread/delete actions, and CSV, TXT, HTML, and XML import/export.
- Restored scheduled SMS, push-channel settings, live logs, account settings, system information, and the local Swagger UI entry.
- Improved responsive layouts for desktop, tablet, and phone screens, including the mobile navigation header, theme switch, and backend heartbeat indicator.
- Standardized pill controls, tab overflow menus, dropdowns, dialogs, spacing, and light/black-dark themes.
- Added local demo data for testing device, network, proxy, SMS, task, and log states without exposing personal information.

### Compatibility

- No persistent-data or configuration migration is required from 2.1.0.
- Full Docker images are published for amd64, arm64/aarch64, and armv7.
- Standalone Go gateway binaries are published for amd64, arm64/aarch64, armv7, and 386 with `SHA256SUMS`.
- The 386 gateway still requires a separately supplied compatible modem core on `127.0.0.1:7576`.
