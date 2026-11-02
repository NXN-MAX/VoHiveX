# Experimental VoHiveX IMS engine

A separate Go service for real SIM-backed VoWiFi call-control and RTP recording
experiments. It is not enabled by the production container entrypoint.

The engine authenticates to an ePDG through the host Internet connection, runs
an IP stack entirely in the process, and registers with the operator's P-CSCF.
It does not create host routes, a TUN interface, or a cellular Internet session.
SIP supports TCP stream framing and UDP transactions. Negotiated `ipsec-3gpp`
security supports AES-CBC or null encryption with HMAC-SHA1-96/HMAC-MD5-96.
Unsupported algorithms fail closed. GSM/UMTS subscriber keys remain on the SIM;
AKA output is held in process memory and is not written to logs.

## Current verification

- Native macOS ARM64 build and Linux AMD64 cross-build succeed.
- The macOS IOKit transport recognizes the original `2ca3:4006` USB identity.
- Real ePDG authentication and IMS registration succeeded on the tested SIM.
- The tested operator selected AES-CBC; a null-only offer was rejected.
- A real outgoing test received G.711 early-media RTP, then an operator `480`.
  This does **not** prove that the recipient's handset rang or that a call connected.
- Incoming calls, SMS/USSD parity, browser microphone/playback integration,
  AMR/AMR-WB codecs, long-duration rekey behavior, and production gateway
  integration are not yet validated. Do not replace a production IMS runtime.

## Build

Use Go 1.26.3 or newer. macOS additionally needs the Apple command-line compiler
and SDK for the IOKit bridge; no third-party system USB driver is needed.

```sh
go test -race ./cmd/vohivex-ims
go build -o vohivex-ims ./cmd/vohivex-ims
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o vohivex-ims-linux-amd64 ./cmd/vohivex-ims
```

## Controlled local hardware debugging

Only one program may own a modem's AT interface. Stop competing modem programs
first. The direct USB mode refuses to start unless the modem is already in
`CFUN=4` with no cellular attachment. Arrange that state only when there is no
active cellular call; this application does not silently switch radio modes.
Read back `CFUN`, `CGATT`, and `CGACT` before and after a hardware test.

```sh
./vohivex-ims -usb-local -device usb-local \
  -mcc HOME_MCC -mnc HOME_MNC \
  -socket ./private/control.sock -recordings ./private/recordings -probe
```

Use the SIM's actual home MCC/MNC, not the visited network's values. `-probe`
performs registration only and exits after the first successful registration.
It does not dial a number. Omit `-probe` to keep the experimental service running.

For container tests, use `-core http://127.0.0.1:7576`, `-config` pointing at the
existing core YAML, and the actual `-device` ID. The original IMS owner and
cellular data must be disabled before starting the experiment. The engine reads
existing credentials; it never replaces the core configuration.

## Private control socket

The Unix socket is created with mode `0600`. There is no externally exposed HTTP
listener. Paths below are relative to that socket:

- `GET /status`: real tunnel, registration, call state and RTP counters.
- `POST /calls` with `{"number":"<E.164 number>"}`: places a real call.
- `POST /calls/{id}/hangup`: explicitly ends a call.
- `POST /calls/{id}/recording/start`: starts recording negotiated media.
- `POST /calls/{id}/recording/stop`: finalizes a recording.

The service does not automatically redial an uncertain call. Early media and
network alerting are distinguished from an answered call. Recording writes
8 kHz stereo PCM WAV (uplink left, downlink right) with a shared timeline. A
recording with no received RTP is rejected and its empty file is removed.

`VOHIVEX_IMS_TRACE_HEADERS=1` enables opt-in registration-header diagnostics;
authorization headers and long numeric subscriber identifiers are redacted.
Do not enable unredacted upstream protocol logging.

See [NOTICE.md](NOTICE.md) and the included licenses for attribution and AGPL
source-distribution requirements. The main application's license does not
replace the independent engine's license.
