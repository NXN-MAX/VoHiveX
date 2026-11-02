# Optional Mac cellular uplink for container Network Debug

A USB modem attached to a Mac is not a NAS-local USB device. An explicit,
authenticated HTTP relay can provide its cellular data connection to Network
Debug without pretending that the modem is physically attached to the NAS.

The operator configures up to four relay choices in
`$VOHIVEX_DATA/network-debug-relay.json`, permissions **0600**:

```json
{
  "relays": [
    {
      "id": "mac-cellular-direct",
      "name": "Mac cellular — direct",
      "interface": "Mac cellular interface",
      "proxy_url": "http://USER:URL_ENCODED_PASSWORD@MAC_LAN_IP:PORT"
    }
  ]
}
```

Replace every placeholder, use a dedicated random password, and allow only the
NAS address at the relay. No relay is configured by default. This private file
must never be committed. Only literal private/loopback relay addresses with
explicit ports and authentication are accepted. The device API omits relay
credentials and only lists reachable relay listeners. A reachable listener does
not prove Internet/carrier connectivity; the browsing request verifies that.

On the Mac, configure the relay's **outbound interface** to the modem's actual
ECM interface. Do not change the system default network, DNS or existing VPN.
Keep all relay DNS and target connections on that interface, reject private and
reserved destination addresses (including resolved hostnames), and fail closed
when the modem is disconnected. Converting a compatible QDC507 from QMI to ECM
requires an idle modem, a saved USBNET value, readback verification and reboot.
Preserve its USB ID, APN, IMS and SIM configuration and restore the original
USBNET mode after the test. Host-network setup may require local administrator
permission. None of this installs dependencies on the NAS host.

For the **container's built-in Mihomo** to use this cellular uplink, place a
separate Mihomo configuration at
`$VOHIVEX_DATA/network-debug-mihomo/config.json` (0600, parent directory 0700).
The gateway supervises this optional instance of its bundled Mihomo binary.
Use a loopback-only authenticated listener; set the selected subscription node's
`dialer-proxy` to a dedicated HTTP proxy pointing to the Mac uplink. Configure a
second relay choice pointing to that loopback listener. This produces:

```text
Browser → NAS Network Debug → bundled Mihomo → Mac HTTP relay
        → cellular modem → subscription node → requested website
```

The existing managed proxy, subscriptions and VoWiFi country rules remain
independent. Without the private optional config, no extra proxy process starts.
Relay failure has no automatic direct/NAS-default-network fallback. Existing
Network Debug authentication, session expiry, URL checks and browser sandboxing
still apply. Remote relay software must also enforce destination restrictions.

Network Debug counters measure HTTP content passing through the browsing
session, not modem billing totals, DNS, TLS, proxy framing or other applications.
A `204` connectivity response correctly transfers zero response-body bytes;
a full web-page test should show nonzero content traffic. Some sites reject
embedded/re-written browsing or require a full browser security context.
