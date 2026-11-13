"""Extend the pinned modem core's existing AT fallback to DJI's EC25 USB ID.

The original QMI/MBIM paths and Quectel/Qualcomm vendor checks stay intact.
Only the exact DJI pair gets the existing fallback scan/worker lifecycle.
Branches stay inside the original Go function: no calls or ABI/frame changes.
"""

import hashlib
import struct


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def _relative(target: int, origin: int, bits: int, scale: int = 1) -> int:
    delta = target - origin
    require(delta % scale == 0, "Unaligned branch target")
    delta //= scale
    require(-(1 << (bits - 1)) <= delta < (1 << (bits - 1)), "Branch out of range")
    return delta & ((1 << bits) - 1)


def _guard(arch: str, address: int, accept: int, reject: int) -> tuple[bytes, bytes]:
    code = bytearray()
    if arch == "amd64":

        def jump(op, target):
            size = len(op) + 4
            code.extend(
                op
                + struct.pack("<I", _relative(target, address + len(code) + size, 32))
            )

        for vendor in (0x2C7C, 0x05C6):
            code.extend(b"\x66\x81\xf9" + struct.pack("<H", vendor))  # CMP CX, VID
            jump(b"\x0f\x84", accept)
        code.extend(b"\x66\x81\xf9\xa3\x2c")
        jump(b"\x0f\x85", reject)
        code.extend(b"\x66\x81\x7c\x24\x26\x06\x40")  # CMP WORD [SP+0x26], 4006
        jump(b"\x0f\x84", accept)
        jump(b"\xe9", reject)
        padding = b"\xcc"
    elif arch == "arm64":

        def instruction(word):
            code.extend(struct.pack("<I", word))

        def branch(target, condition=None):
            delta = _relative(
                target, address + len(code), 26 if condition is None else 19, 4
            )
            instruction(
                0x14000000 | delta
                if condition is None
                else 0x54000000 | (delta << 5) | condition
            )

        for vendor in (0x2C7C, 0x05C6):
            instruction(0xD2800004 | (vendor << 5))  # MOV X4, VID
            instruction(0x6B04005F)  # CMP W2, W4
            branch(accept, 0)
        instruction(0xD2800004 | (0x2CA3 << 5))
        instruction(0x6B04005F)
        branch(reject, 1)
        instruction(0x79405FE3)  # LDRH W3, [SP+0x2e] (saved PID)
        instruction(0xD2800004 | (0x4006 << 5))
        instruction(0x6B04007F)  # CMP W3, W4
        branch(accept, 0)
        branch(reject)
        padding = struct.pack("<I", 0xD503201F)  # NOP, unreachable
    elif arch == "armv7":

        def instruction(word):
            code.extend(struct.pack("<I", word))

        def movw(value):
            instruction(0xE300B000 | ((value & 0xF000) << 4) | (value & 0xFFF))

        def branch(target, condition):
            delta = _relative(target, address + len(code) + 8, 24, 4)
            instruction((condition << 28) | 0x0A000000 | delta)

        for vendor in (0x2C7C, 0x05C6):
            movw(vendor)
            instruction(0xE152000B)  # CMP R2, R11
            branch(accept, 0)
        movw(0x2CA3)
        instruction(0xE152000B)
        branch(reject, 1)
        instruction(0xE1DD23B6)  # LDRH R2, [SP+54] (saved PID)
        movw(0x4006)
        instruction(0xE152000B)
        branch(reject, 1)  # Equal falls through to the original accept path.
        padding = struct.pack("<I", 0xE7F000F0)  # UDF, unreachable
    else:
        raise ValueError("Unsupported modem-core architecture")
    return bytes(code), padding


def patch_modem_discovery(data: bytearray, arch: str, spec: dict) -> dict:
    """Validate the complete pinned function and replace only its vendor guard."""
    machines = {"amd64": (2, 62), "arm64": (2, 183), "armv7": (1, 40)}
    require(arch in machines, "Unsupported modem-core architecture")
    elf_class, machine = machines[arch]
    require(len(data) >= (64 if elf_class == 2 else 52), "Truncated ELF header")
    require(
        data[:4] == b"\x7fELF" and data[4:6] == bytes((elf_class, 1)),
        "Unexpected ELF format",
    )
    require(
        struct.unpack_from("<H", data, 18)[0] == machine, "ELF architecture mismatch"
    )
    start, size = spec["offset"], spec["size"]
    require(0 <= start < start + size <= len(data), "Invalid function range")
    require(
        hashlib.sha256(data[start : start + size]).hexdigest() == spec["sha256"],
        "Unsupported native scanner function",
    )
    offset, length = spec["guard_offset"], spec["guard_size"]
    address = spec["address"] + offset - start
    require(
        start <= offset < offset + length <= start + size, "Invalid vendor guard range"
    )
    require(
        data[offset : offset + length] == bytes.fromhex(spec["guard_hex"]),
        "Native vendor guard changed",
    )
    for target in (spec["accept"], spec["reject"]):
        require(
            spec["address"] <= target < spec["address"] + size,
            "Branch leaves original function",
        )
        require(
            not address <= target < address + length, "Branch enters replacement bytes"
        )
    code, padding = _guard(arch, address, spec["accept"], spec["reject"])
    require(
        len(code) <= length and (length - len(code)) % len(padding) == 0,
        "Vendor guard does not fit",
    )
    data[offset : offset + length] = code + padding * (
        (length - len(code)) // len(padding)
    )
    return {
        "usb_id": "2ca3:4006",
        "ec25_reference": "2c7c:0125",
        "function": "discoverFallbackOne",
        "function_sha256": spec["sha256"],
        "mechanism": "native AT fallback; original USB identity retained",
        "guard_offset": offset,
        "guard_size": length,
        "guard_sha256": hashlib.sha256(data[offset : offset + length]).hexdigest(),
    }
