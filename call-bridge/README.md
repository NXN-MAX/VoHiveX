# VoHiveX native browser telephone bridge

This optional macOS helper connects the VoHiveX phone page to the tested QDC507
USB modem. It supports cellular call control, browser microphone/speaker audio,
mute, listen-only mode, and two-channel WAV recording. It does **not** implement
VoWiFi media or turn an existing VoWiFi device into a cellular device.

## Build

Requires Go, a C toolchain, macOS CoreAudio frameworks and libusb development
files exposed through `pkg-config`. The helper is a separate Go module; normal
Linux gateway/container builds do not need these macOS dependencies.

```sh
cd call-bridge
go test ./...
go build -o vohivex-call-bridge .
```

Do not install dependencies on a NAS host for this macOS path. Modem-side kernel
modules are not bundled. Obtain the three exact runtime artifacts described in
[NOTICE.md](NOTICE.md) from the pinned upstream revision, check their licenses,
and place them in a private runtime directory. The helper checks their sizes
and SHA-256 hashes before changing the USB configuration. Support is restricted
to the tested USB ID `2ca3:4006`, original interface composition, kernel 3.18.44
and audio layout; a matching kernel string alone does not establish compatibility.

## Gateway configuration

Set these environment variables when launching the regular VoHiveX gateway:

| Variable | Purpose |
| --- | --- |
| `VOHIVEX_CALL_BRIDGE` | Absolute path to the compiled helper |
| `VOHIVEX_USB_LOCATION` | Physical USB location ID of the intended modem; no default |
| `VOHIVEX_CALL_BRIDGE_STATE` | Absolute path to a private rollback/lock directory |
| `VOHIVEX_CALL_RUNTIME_DIR` | Directory containing the three pinned runtime files |
| `VOHIVEX_LOCAL_CALLS_ONLY=1` | Optional isolated local test: only expose the local modem to phone APIs; suspend scheduler, delivery and proxy maintenance workers; use the upstream account agreement |
| `VOHIVEX_LISTEN_HOST=127.0.0.1` | Bind a local test instance to loopback |

Continue configuring the gateway's upstream authentication endpoint, data and
asset directories normally. Use a separate local data directory for development.
The existing upstream administrator login remains required. No default test
password or authentication bypass is introduced.

## Phone-page flow

1. Select **QDC507 · 本机 USB**. The hardware must be idle before preparation.
2. Choose **准备音频**. Preparation saves the original USB composition privately,
   enables ADB and USB Audio while preserving the USB ID, reboots once, loads
   verified drivers into modem RAM, calibrates and starts the voice route.
   It never changes APN, IMS, SIM profiles or host cellular Internet routes.
3. Choose **连接网页音频** and grant microphone access to this local application.
   HTTPS or localhost is required. **仅收听** connects without a microphone.
4. Dial or answer using the existing phone controls. **开始录音** becomes available
   for a connected call with an active audio stream. WAV left channel is remote
   audio; right channel is browser uplink. Recordings stay in the private VoHiveX
   data directory and are played through authenticated recording endpoints.
5. Disconnecting audio, navigating away or closing the page releases the browser
   microphone and saves an active recording. It **does not hang up** the call.
   Use the separate hangup control when you intend to end the call.
6. Once idle, disconnect audio and choose **恢复接口**. This restores the saved USB
   composition and reboots to clear temporary kernel modules and runtime state.
   Do not unload the voice modules with `rmmod`; use this controlled reboot.

An interrupted preparation retains its rollback snapshot. Reconnect the same
modem at the same physical location and restore before preparing again. If a
call is active, restoration is refused. Do not delete the snapshot to override
this check. Temporary module loading can require physical unplug/replug recovery.

## Audio protocol and verification

The gateway authenticates a short-lived, single-use WebSocket ticket without
putting credentials in URLs. Audio is mono PCM16LE at 8 kHz in each direction.
Connections are exclusive, frame sizes and buffers are bounded, and a ready
message is sent only after real capture frames arrive. Browser microphone
capture begins only from the connect action; the native helper opens only the
USB audio endpoints matching the specified physical modem.

```sh
go test ./cmd/vohivex-gateway
node --test web/tests/callAudio.test.mjs
```

Run those commands from the repository root. Tests cover authentication,
origin/ticket isolation, duplex transport, disconnect behavior, recording
format/permissions, resampling and mute. Real incoming/outgoing calls, remote
speech, uplink audibility, recording playback and USB restoration require a
compatible modem and an operator-authorized test number. Automated tests do
not establish carrier support or two-way voice quality.
