# VoHiveX IMS engine

This independent process is licensed under AGPL-3.0. It communicates with the
existing gateway/core using authenticated HTTP and a private Unix socket.
It is experimental until real operator, messaging and media checks pass.

- `third_party/vowifi-go`: boa-z/vowifi-go, commit
  `1e9c6e6adbfcd9667695149d5ecb0f71cd062f07`, AGPL-3.0. The historical module
  name is preserved for compatibility. Source files are vendored for reproducible builds.
- `third_party/swu-go`: wstgpt/VoHive-VoWiFi-IMS, commit
  `7cf6929b8c082a6c64c023415c5bba2a71d951b8`, swu-go subtree, MIT,
  copyright 2026 iniwex5. Uses only its in-process ESP dataplane. The private copy removes unused
  kernel/TUN integration and separates Linux-only serial and error-queue code
  so the same packet dataplane builds on macOS.
- gVisor netstack: Apache-2.0, version pinned in go.mod.
- sipgo: BSD-2-Clause, version pinned in go.mod.

No VoCat source or restricted runtime is included. Modifications made for
VoHiveX in October 2026 add the independent control service and media pipeline.
Distributed binaries must be accompanied by the corresponding engine source,
including dependency sources and build instructions. Do not relicense this
service under the main application's noncommercial license.

- `internal/mavousb`: MaVo CModemBridge, MIT, commit
  `0443dfdaf8aec086fd76ba2ee9152fd908114524`, copyright 2026 MaVo
  contributors. The original license is retained in that directory. The Go
  wrapper accesses only the AT interface; it does not change USB IDs, install
  a driver, or start cellular data.
