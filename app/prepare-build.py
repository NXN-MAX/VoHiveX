#!/usr/bin/env python3
"""Prepare pinned build inputs from a source-only checkout; never installs host packages."""
import gzip
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
UPSTREAM = 'https://raw.githubusercontent.com/Orson-Yan/Vohive-155/2cc83765f694b4a821489af9178c93d8aca2a489/release/vohive_v1.5.5-10-gf9eb85d_linux_amd64'


def fetch(url, expected):
    with urllib.request.urlopen(url, timeout=120) as response:
        data = response.read()
    if hashlib.sha256(data).hexdigest() != expected:
        raise SystemExit('Downloaded input failed SHA256 verification')
    return data


def main():
    for tool in ('node', 'npm', 'upx'):
        if not shutil.which(tool):
            raise SystemExit(f'Build machine requires {tool}; use the complete deployment package on a runtime host.')
    manifest = json.loads((ROOT / 'app/patch-manifest.json').read_text())
    proxy = next(x for x in json.loads((ROOT / 'app/proxy/vendor/manifest.json').read_text())['binaries'] if x['arch']=='linux-amd64-compatible')
    subprocess.run(['npm', 'ci', '--prefix', 'app/build-tools', '--ignore-scripts', '--no-audit', '--no-fund'], cwd=ROOT, check=True)
    with tempfile.TemporaryDirectory() as folder:
        original = Path(folder) / 'upstream-amd64'
        original.write_bytes(fetch(UPSTREAM, manifest['input_sha256']))
        subprocess.run(['python3', '-B', 'app/patch-release.py', '--original', str(original)], cwd=ROOT, check=True)
    binary = gzip.decompress(fetch(proxy['url'], proxy['archive_sha256']))
    if hashlib.sha256(binary).hexdigest() != proxy['sha256']:
        raise SystemExit('Mihomo binary failed SHA256 verification')
    target = ROOT / 'app/proxy/vendor/mihomo-linux-amd64-compatible'
    temp = target.with_suffix('.tmp')
    temp.write_bytes(binary)
    temp.chmod(0o755)
    temp.replace(target)
    subprocess.run(['python3', '-B', 'app/scheduler/build-assets.py'], cwd=ROOT, check=True)
    subprocess.run(['python3', '-B', 'app/verify-build.py'], cwd=ROOT, check=True)


if __name__ == '__main__':
    main()
