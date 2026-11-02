# Native call bridge

The USB AT and ADB transport files (`usbat_*`, `adb_*`, `qadb_key.go`) derive
from MacCellular commit `e443b233edaa3671629f69182fe18bb88464d1c5` and retain
its PolyForm Noncommercial license in `third_party/maccellular/LICENSE`.
Changes add a bounded CLI, device ownership, private rollback snapshots and
authenticated browser audio integration through the VoHiveX gateway.

The optional modem-side runtime is independently fetched by its operator from
MaVo commit `0443dfdaf8aec086fd76ba2ee9152fd908114524`. Its binaries are not
bundled or redistributed here. Preparation checks exact SHA-256 identities.
The two kernel modules are GPL-2.0; the user-space helper is covered by MaVo's
MIT license. Runtime orchestration follows MaVo's published preparation and
calibration sequence. The upstream report does not reproduce the exact pinned
binaries, so kernel release matching does not prove ABI compatibility.

CoreAudio streaming, gateway integration and browser audio code are original
VoHiveX contributions. No computer microphone is opened by this helper: uplink
audio is provided only by the browser after its microphone permission prompt.
