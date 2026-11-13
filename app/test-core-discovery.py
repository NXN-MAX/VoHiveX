"""Run the actual packaged core against isolated USB fixtures, on every image arch.

No host USB devices or Docker socket are mounted into the test container.
The sole AT character device is a fresh pseudo-terminal with fictional identity.
"""

import argparse
import json
import os
import pty
import select
import subprocess
import sys
import tempfile
import threading
import time
import tty
import urllib.error
import urllib.request
import uuid
from pathlib import Path


def docker(*args, check=True):
    result = subprocess.run(
        ["docker", *args], check=False, text=True, capture_output=True, timeout=90
    )
    if check and result.returncode:
        raise RuntimeError(result.stderr[-3000:])
    return result.stdout.strip()


def fixture(root, vendor, product, mode):
    sys = root / "sys"
    usb = sys / "devices/test/usb1/1-1"
    usb.mkdir(parents=True)
    for name, value in {
        "idVendor": vendor,
        "idProduct": product,
        "bConfigurationValue": "1",
        "bNumInterfaces": "6",
    }.items():
        (usb / name).write_text(value)
    devices = sys / "bus/usb/devices"
    devices.mkdir(parents=True)
    # The relative symlink resolves inside /sys after the bind mount.
    (devices / "1-1").symlink_to("../../../devices/test/usb1/1-1")
    for number in (0, 2, 3, 4, 5):
        interface = usb / f"1-1:1.{number}"
        interface.mkdir()
        (devices / interface.name).symlink_to(
            "../../../devices/test/usb1/1-1/" + interface.name
        )
        serial = number in (0, 2, 3)
        driver = (
            "option"
            if serial
            else {"ecm": "cdc_ether", "qmi": "qmi_wwan", "mbim": "cdc_mbim"}[mode]
        )
        cls = "ff" if serial or mode == "qmi" else ("02" if number == 4 else "0a")
        subclass = (
            "06"
            if mode == "ecm" and number == 4
            else ("0e" if mode == "mbim" and number == 4 else "00")
        )
        for name, value in {
            "bInterfaceNumber": f"{number:02x}",
            "bInterfaceClass": cls,
            "bInterfaceSubClass": subclass,
            "bInterfaceProtocol": "00",
        }.items():
            (interface / name).write_text(value)
        (interface / "driver").symlink_to("/fixture/drivers/" + driver)
        if serial:
            (interface / f"ttyUSB{number}/tty/ttyUSB{number}").mkdir(parents=True)
        if number == 4:
            (interface / "net/wwan_fixture0").mkdir(parents=True)
            if mode != "ecm":
                (interface / "usbmisc/cdc-wdm0").mkdir(parents=True)
    return sys


class ATFixture:
    def __init__(self):
        self.master, self.slave = pty.openpty()
        tty.setraw(self.slave)
        self.path = os.ttyname(self.slave)
        self.stop = threading.Event()
        self.commands = []
        self.thread = threading.Thread(target=self.run, daemon=True)
        self.thread.start()

    def run(self):
        pending = b""
        while not self.stop.is_set():
            if not select.select([self.master], [], [], 0.1)[0]:
                continue
            try:
                data = os.read(self.master, 1024)
            except OSError:
                continue
            pending += data
            while b"\r" in pending:
                line, pending = pending.split(b"\r", 1)
                command = line.decode(errors="replace").strip()
                if not command:
                    continue
                self.commands.append(command)
                if command in ("AT+CGSN", "AT+GSN", "AT+CGSN=1"):
                    body = "1" * 15
                elif command in ("AT", "ATE0"):
                    body = ""
                elif command in ("ATI", "AT+CGMM"):
                    body = "EC25"
                elif command == "AT+CGMI":
                    body = "Quectel"
                elif command == "AT+CGMR":
                    body = "FIXTURE"
                else:
                    os.write(self.master, b"\r\nERROR\r\n")
                    continue
                os.write(self.master, ("\r\n" + body + "\r\nOK\r\n").encode())

    def close(self):
        self.stop.set()
        self.thread.join(timeout=2)
        os.close(self.master)
        os.close(self.slave)


def request(base, path, token=None, data=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(
        base + path,
        headers=headers,
        data=None if data is None else json.dumps(data).encode(),
    )
    with urllib.request.urlopen(req, timeout=20) as response:
        return json.load(response)


def run_case(image, platform, vendor, product, mode, expected, core_binary=None):
    name = "vohivex-discovery-test-" + uuid.uuid4().hex[:12]
    at = ATFixture()
    try:
        with tempfile.TemporaryDirectory(prefix="vohivex-usb-fixture-") as directory:
            root = Path(directory)
            sys = fixture(root, vendor, product, mode)
            config = root / "config"
            config.mkdir()
            (config / "config.yaml").write_text(
                'server:\n  port: "0.0.0.0:7576"\nweb:\n  username: admin\n  password: admin\ndevices: []\n'
            )
            override = (
                []
                if core_binary is None
                else [
                    "--mount",
                    f"type=bind,source={core_binary},target=/app/vohive,readonly",
                ]
            )
            docker(
                "run",
                "--detach",
                "--rm",
                "--name",
                name,
                "--platform",
                platform,
                "--read-only",
                "--cap-drop",
                "ALL",
                "--cap-add",
                "DAC_OVERRIDE",
                "--tmpfs",
                "/tmp:rw,nosuid,nodev,size=32m",
                "--tmpfs",
                "/app/data:rw,nosuid,nodev,size=32m",
                "--tmpfs",
                "/app/logs:rw,nosuid,nodev,size=16m",
                "--mount",
                f"type=bind,source={sys},target=/sys,readonly",
                "--mount",
                f"type=bind,source={config},target=/app/config",
                "--mount",
                f"type=bind,source={at.path},target=/dev/ttyUSB2",
                "--publish",
                "127.0.0.1::7576",
                *override,
                "--entrypoint",
                "/app/vohive",
                image,
                "-c",
                "/app/config/config.yaml",
            )
            port = json.loads(docker("inspect", name))[0]["NetworkSettings"]["Ports"][
                "7576/tcp"
            ][0]["HostPort"]
            base = "http://127.0.0.1:" + port
            token = None
            deadline = time.monotonic() + 60
            while time.monotonic() < deadline:
                try:
                    token = request(
                        base,
                        "/api/auth/login",
                        data={"username": "admin", "password": "admin"},
                    )["token"]
                    break
                except (urllib.error.URLError, OSError, KeyError):
                    time.sleep(0.5)
            if not token:
                raise RuntimeError(
                    "Core did not become ready: "
                    + docker("logs", name, check=False)[-2000:]
                )
            rows = request(base, "/api/devices/discovered?with_imei=0", token).get(
                "devices", []
            )
            if len(rows) != (1 if expected else 0):
                raise AssertionError(
                    f"{vendor}:{product}/{mode}: unexpected native discovery count {len(rows)}"
                )
            if expected:
                row = rows[0]
                if row.get("at_port") != "/dev/ttyUSB2":
                    raise AssertionError("Core selected a diagnostic/GNSS port")
                if mode == "ecm" and row.get("driver_name") != "cdc_ether":
                    raise AssertionError("ECM driver identity changed")
                if row.get("vendor_id") != int(vendor, 16) or row.get(
                    "product_id"
                ) != int(product, 16):
                    raise AssertionError("Core changed the real USB identity")
                if mode == "ecm":
                    rows = request(
                        base, "/api/devices/discovered?with_imei=1", token
                    ).get("devices", [])
                    if len(rows) != 1 or rows[0].get("imei") != "1" * 15:
                        raise AssertionError("Native ECM identity lookup failed")
            print(
                f"PASS {platform} {vendor}:{product} {mode} native core discovery",
                flush=True,
            )
    finally:
        docker("rm", "--force", name, check=False)
        at.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", required=True)
    parser.add_argument(
        "--core-binary",
        type=Path,
        help="Optional local core binary for isolated pre-release testing",
    )
    args = parser.parse_args()
    if sys.platform != "linux":
        parser.error("Run core integration fixtures on a Linux Docker host")
    for vendor, product, mode, expected in [
        ("2ca3", "4006", "ecm", True),
        ("2ca3", "4006", "qmi", True),
        ("2c7c", "0125", "ecm", True),
        ("05c6", "9003", "ecm", True),
        ("2c7c", "0125", "mbim", True),
        ("2ca3", "4007", "ecm", False),
        ("1234", "4006", "ecm", False),
    ]:
        run_case(
            args.image, args.platform, vendor, product, mode, expected, args.core_binary
        )


if __name__ == "__main__":
    main()
