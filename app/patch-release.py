#!/usr/bin/env python3
"""Version-locked amd64 customization. Original release is never overwritten."""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
manifest = json.loads((root / 'app/patch-manifest.json').read_text())
parser = argparse.ArgumentParser(description='Rebuild the version-locked amd64 release from an explicit upstream original.')
parser.add_argument('--original', required=True, type=Path, help='Path to the original upstream amd64 binary; SHA256 must match patch-manifest.json')
original = parser.parse_args().original.resolve()
if not original.is_file():
    parser.error('Original binary does not exist')
if original == (root/'release/vohive-dji-amd64').resolve():
    parser.error('The output release cannot be used as the upstream original')
sha = lambda data: hashlib.sha256(data).hexdigest()
assert sha(original.read_bytes()) == manifest['input_sha256'], 'Unsupported upstream binary'
with tempfile.TemporaryDirectory() as directory:
    unpacked = Path(directory) / 'vohive'
    subprocess.run(['upx', '-d', str(original), '-o', str(unpacked)], check=True)
    data = bytearray(unpacked.read_bytes())
assert sha(data) == manifest['unpacked_sha256'], 'Unexpected decompressed binary'

# Go amd64 ABIInternal: method receiver AX, *gin.Context BX. Tail-call the
# existing AbortWithStatus(ctx, 410) function, which owns its stack checks.
for handler in manifest['handlers']:
    start, size = handler['offset'], handler['size']
    assert sha(data[start:start + size]) == handler['original_sha256']
    stub = b'\x48\x89\xd8\xbb' + struct.pack('<I', 410)
    stub += b'\xe9' + struct.pack('<i', manifest['abort_with_status'] - (handler['address'] + 13))
    assert len(stub) == 13
    data[start:start + size] = stub + b'\xcc' * (size - len(stub))

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
target = out / 'vohive-dji-amd64'
target.write_bytes(data)
target.chmod(0o755)
report = {'original_sha256': manifest['input_sha256'], 'patched_sha256': sha(data),
          'disabled_http_handlers': [h['name'] for h in manifest['handlers']],
          'http_status': 410, 'api_docs': '/api/docs', 'platform': 'linux/amd64'}
(out / 'patch-result.json').write_text(json.dumps(report, indent=2) + '\n')
print(json.dumps(report, indent=2))
