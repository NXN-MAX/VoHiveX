# Proxy Management

## Import and Connect

1. Open **Proxy Management → Subscriptions and Nodes**. Add HTTPS subscription URLs, one per line, individual node URLs, or Clash YAML. PNG, JPG, and WebP QR images are also accepted.
2. After confirming the import, expand the subscription, select a node, and click **Use**. Importing does not enable the proxy.
3. **Connect** uses the last selected node. **Disconnect** stops the built-in proxy, and linked country rules fall back to direct connections.
4. Update or delete subscriptions as needed. Before deleting a node in use, disconnect or select another node.

Subscriptions are collapsed by default, and node lists scroll within a fixed height. **Test all** displays each node's HTTPS latency without changing the active egress. Stopping a test waits for the current request and cancels the remaining queue. The latency badge displays at most 999 ms; hover to see the actual value. No badge is shown while disconnected or after a failed test.

QR images are decoded only in the browser and are never uploaded or retained in a persistent cache. Image data is cleared after recognition. The recognized subscription or node configuration is saved only after the user confirms the import.

## VoWiFi Roaming Proxy

- **Subscription nodes · Built-in proxy** always exists and cannot be deleted. Its SOCKS5 endpoint is fixed at `127.0.0.1:17890`.
- Connecting or selecting a subscription node enables the built-in entry. Add country rules for it under **VoWiFi roaming proxy**.
- Countries are matched against the SIM's MCC. Only enabled and linked rules use the assigned proxy. Disabled rules fall back to a direct connection.
- Changing the built-in node affects every linked country. Existing proxy connections may need to be rebuilt, and VoWiFi may need to register again.
- Manual SOCKS5 proxies can be configured separately. VoWiFi requires UDP Associate support; an HTTPS latency test does not prove that UDP or IMS registration works.

## Local Outbound Proxy

New instances are disabled by default. Enter the bound device, listening address, port, and credentials, save the instance, and then enable it from the list. Disabling the switch stops the instance. Review the returned error and logs when startup fails.

## IP Addresses

- When VoWiFi is not in use, a device card displays only the IP obtained by the SIM. It does not substitute the host's public IP.
- When VoWiFi is active, egress is determined from enabled country rules and proxy configuration. The UI reports unavailable when it cannot determine the egress.
- **Device public IP** uses the default non-cellular egress. **Subscription proxy egress IP** queries only through the built-in SOCKS5 proxy and never falls back to a direct connection.
- The interface can display IPv6 text. Long values are truncated; hover over a device-card value to see it in full, or click an IP on the proxy page to copy it.
- External discovery currently uses `api.ipify.org` and falls back to `ipv4.icanhazip.com`. Only public IPv4 responses are accepted. IPv6 text support in the interface does not mean that public IPv6 discovery is implemented.
- Queries contain no subscription, SIM, or SMS data. Results are cached for 60 seconds, and manual refresh is rate-limited to once every 5 seconds.

## Limits and Storage

| Item | Value |
| --- | --- |
| SOCKS5 | `127.0.0.1:17890` |
| Control API | `127.0.0.1:17891`, protected by a random secret |
| Persistent directory | `data/managed-proxy/` |
| Sources | Up to 20 |
| Nodes | Up to 1,000 per source and 1,000 total |
| Subscription download | HTTPS, up to 4 MB; private and reserved addresses are rejected |
| Latency target | `https://www.gstatic.com/generate_204` |

Supported inputs include Clash YAML, Base64 subscriptions, and common node URIs for VMess, VLESS, AnyTLS, Trojan, Shadowsocks, Hysteria2, TUIC, and SOCKS5. External Shadowsocks plugins are not supported. Core configuration is validated before import, and an invalid configuration is not applied.

TUN, DNS, routing rules, external controllers, and rule-download settings from a subscription are not imported. Subscription hostnames must resolve to genuine public addresses. Subscription and node secrets are stored in configuration files with mode 0600 and must never be added to images or public deployment packages. Updates, imports, and node changes reload the core; a failed reload attempts to restore the previous configuration.

## API

- `GET /api/managed-proxy`: status, sources, redacted nodes, and version.
- `POST /api/managed-proxy/{import,refresh,delete,select,pause,attach,test}`: management operations. Write operations include the current `version`; tests use `node_id`.
- `GET /api/managed-proxy/public-ip`: egress IP; add `?refresh=1` for manual refresh.

The API uses the existing login authentication. See [THIRD-PARTY.md](THIRD-PARTY.md) for third-party licenses.
