#!/usr/bin/env python3
"""Version-locked multi-architecture customization. Original release is never overwritten."""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description='Rebuild the version-locked release from an explicit upstream original.')
parser.add_argument('--arch', choices=('amd64', 'arm64', 'armv7'), default='amd64')
parser.add_argument('--original', required=True, type=Path, help='Path to the original upstream binary; SHA256 must match patch-manifest.json')
args = parser.parse_args()
arch = args.arch
manifest_name = 'patch-manifest.json' if arch == 'amd64' else f'patch-manifest-{arch}.json'
manifest = json.loads((root / 'app' / manifest_name).read_text())
original = args.original.resolve()
if not original.is_file():
    parser.error('Original binary does not exist')
if original == (root/f'release/vohivex-{arch}').resolve():
    parser.error('The output release cannot be used as the upstream original')
sha = lambda data: hashlib.sha256(data).hexdigest()
assert sha(original.read_bytes()) == manifest['input_sha256'], 'Unsupported upstream binary'
with tempfile.TemporaryDirectory() as directory:
    unpacked = Path(directory) / 'vohive'
    subprocess.run(['upx', '-d', str(original), '-o', str(unpacked)], check=True)
    data = bytearray(unpacked.read_bytes())
assert sha(data) == manifest['unpacked_sha256'], 'Unexpected decompressed binary'

# Replace only SHA256-locked functions. Tail-call the existing Gin method so
# its normal stack-growth prologue runs; no new call frame is introduced.
for handler in manifest['handlers']:
    start, size = handler['offset'], handler['size']
    assert sha(data[start:start + size]) == handler['original_sha256']
    address, target = handler['address'], manifest['abort_with_status']
    if arch == 'amd64':
        # ABIInternal: receiver AX, context BX -> context AX, status BX.
        stub = b'\x48\x89\xd8\xbb' + struct.pack('<I', 410)
        stub += b'\xe9' + struct.pack('<i', target - (address + 13))
        padding = b'\xcc'
    elif arch == 'arm64':
        # ABIInternal: receiver X0, context X1 -> context X0, status X1.
        delta = target - (address + 8)
        assert delta % 4 == 0 and -(1 << 27) <= delta < (1 << 27)
        stub = struct.pack('<III', 0xaa0103e0, 0xd2803341,
                           0x14000000 | ((delta // 4) & 0x03ffffff))
        padding = struct.pack('<I', 0xd4200000)  # BRK
    else:
        # ARMv7 ABI0: receiver SP+4, context SP+8. Reuse those argument slots
        # for AbortWithStatus(context, 410); leave LR and the Go G register intact.
        delta = target - (address + 16 + 8)
        assert delta % 4 == 0 and -(1 << 25) <= delta < (1 << 25)
        stub = struct.pack('<IIIII', 0xe59d0008, 0xe58d0004, 0xe300019a,
                           0xe58d0008, 0xea000000 | ((delta // 4) & 0x00ffffff))
        padding = struct.pack('<I', 0xe7f000f0)  # UDF
    assert len(stub) <= size and (size - len(stub)) % len(padding) == 0
    data[start:start + size] = stub + padding * ((size - len(stub)) // len(padding))

settings = (root / 'app/frontend/Settings.min.js').read_bytes()
for forbidden in ['系统信息', '检查更新', '交流群', 't.me/vohive', '/system/info', '/system/update']:
    assert forbidden.encode() not in settings
assert b'/api/docs' in settings, 'API documentation must remain available'
renames = {}
for asset in manifest['assets']:
    if asset['name'].endswith('.js'):
        name = Path(asset['name']).name.encode()
        prefix, hashed = name[:-12], name[-11:]
        assert name[-12:-11] == b'-' and len(hashed) == 11
        renames[name] = prefix + b'-DJI26003.js'
for asset in manifest['assets']:
    offset, size = asset['offset'], asset['size']
    payload = bytes(data[offset:offset + size])
    assert sha(payload) == asset['sha256']
    if asset['name'].endswith('/Settings-NYS93jSt.js'):
        assert len(settings) <= size
        payload = settings.ljust(size, b' ')
    for old, new in renames.items():
        payload = payload.replace(old, new)
    data[offset:offset + size] = payload
    for offset in asset['hash_offsets']:
        data[offset:offset + 16] = hashlib.sha256(payload).digest()[:16]
# Update same-length embedded filenames as well as references; offsets stay fixed.
for old, new in renames.items():
    assert len(old) == len(new)
    data = data.replace(old, new)

out = root / 'release'
out.mkdir(exist_ok=True)
target = out / f'vohivex-{arch}'
target.write_bytes(data)
target.chmod(0o755)
report = {'original_sha256': manifest['input_sha256'], 'patched_sha256': sha(data),
          'disabled_http_handlers': [h['name'] for h in manifest['handlers']],
          'http_status': 410, 'api_docs': '/api/docs', 'platform': 'linux/' + ('arm/v7' if arch == 'armv7' else arch)}
(out / ('patch-result.json' if arch == 'amd64' else f'patch-result-{arch}.json')).write_text(json.dumps(report, indent=2) + '\n')
print(json.dumps(report, indent=2))
