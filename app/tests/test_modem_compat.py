"""Fail-closed patch validation; test-core-discovery.py executes the real core."""

import hashlib
import importlib.util
import json
import struct
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("modem_compat", APP / "modem_compat.py")
COMPAT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(COMPAT)


def fixture(arch):
    name = "patch-manifest.json" if arch == "amd64" else f"patch-manifest-{arch}.json"
    original = json.loads((APP / name).read_text())["native_modem_discovery"]
    guard = bytes.fromhex(original["guard_hex"])
    data = bytearray(1024)
    data[:6] = b"\x7fELF" + bytes((1 if arch == "armv7" else 2, 1))
    struct.pack_into("<H", data, 18, {"amd64": 62, "arm64": 183, "armv7": 40}[arch])
    data[256 : 256 + len(guard)] = guard
    spec = {
        "offset": 128,
        "size": 768,
        "address": 0x1000,
        "guard_offset": 256,
        "guard_size": len(guard),
        "guard_hex": guard.hex(),
        "accept": 0x1000 + 128 + len(guard),
        "reject": 0x1000 + 640,
    }
    spec["sha256"] = hashlib.sha256(data[128:896]).hexdigest()
    return data, spec


class ModemCompatibility(unittest.TestCase):
    def test_all_arches_change_only_the_locked_guard(self):
        for arch in ("amd64", "arm64", "armv7"):
            with self.subTest(arch=arch):
                data, spec = fixture(arch)
                before = bytes(data)
                report = COMPAT.patch_modem_discovery(data, arch, spec)
                begin, end = (
                    spec["guard_offset"],
                    spec["guard_offset"] + spec["guard_size"],
                )
                self.assertEqual(data[:begin], before[:begin])
                self.assertEqual(data[end:], before[end:])
                self.assertEqual(len(data), len(before))
                self.assertNotEqual(data[begin:end], before[begin:end])
                self.assertEqual(report["usb_id"], "2ca3:4006")

    def test_rejects_corrupted_function_before_any_write(self):
        data, spec = fixture("amd64")
        data[200] ^= 1
        before = bytes(data)
        with self.assertRaisesRegex(ValueError, "Unsupported native scanner"):
            COMPAT.patch_modem_discovery(data, "amd64", spec)
        self.assertEqual(data, before)

    def test_rejects_guard_changes_even_with_updated_function_hash(self):
        data, spec = fixture("amd64")
        data[256] ^= 1
        spec["sha256"] = hashlib.sha256(data[128:896]).hexdigest()
        before = bytes(data)
        with self.assertRaisesRegex(ValueError, "vendor guard changed"):
            COMPAT.patch_modem_discovery(data, "amd64", spec)
        self.assertEqual(data, before)

    def test_rejects_wrong_architecture(self):
        data, spec = fixture("amd64")
        before = bytes(data)
        with self.assertRaisesRegex(ValueError, "architecture mismatch"):
            COMPAT.patch_modem_discovery(data, "arm64", spec)
        self.assertEqual(data, before)

    def test_rejects_branches_outside_function_or_into_guard(self):
        for target in (0x9999, 0x1080):
            data, spec = fixture("amd64")
            spec["accept"] = target
            before = bytes(data)
            with self.assertRaises(ValueError):
                COMPAT.patch_modem_discovery(data, "amd64", spec)
            self.assertEqual(data, before)

    def test_rejects_short_replacement_region(self):
        data, spec = fixture("amd64")
        spec["guard_size"] = 1
        spec["guard_hex"] = data[256:257].hex()
        before = bytes(data)
        with self.assertRaisesRegex(ValueError, "does not fit"):
            COMPAT.patch_modem_discovery(data, "amd64", spec)
        self.assertEqual(data, before)

    def test_rejects_unaligned_arm_target(self):
        data, spec = fixture("arm64")
        spec["accept"] += 1
        before = bytes(data)
        with self.assertRaisesRegex(ValueError, "Unaligned"):
            COMPAT.patch_modem_discovery(data, "arm64", spec)
        self.assertEqual(data, before)

    def test_optimized_python_cannot_disable_upstream_integrity_checks(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "unsupported-core"
            path.write_bytes(b"not a supported upstream binary")
            result = subprocess.run(
                [
                    sys.executable,
                    "-O",
                    str(APP / "patch-release.py"),
                    "--original",
                    str(path),
                ],
                check=False,
                text=True,
                capture_output=True,
            )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Unsupported upstream binary", result.stderr)


if __name__ == "__main__":
    unittest.main()
