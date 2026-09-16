#!/usr/bin/env python3
"""Prepare pinned build inputs from a source-only checkout; never installs host packages."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
UPSTREAM = 'https://raw.githubusercontent.com/Orson-Yan/Vohive-155/2cc83765f694b4a821489af9178c93d8aca2a489/release/vohive_v1.5.5-10-gf9eb85d_linux_{}'


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
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', choices=('all', 'amd64', 'arm64', 'armv7'), default='all')
    arch = parser.parse_args().arch
    requested = ('amd64', 'arm64', 'armv7') if arch == 'all' else (arch,)
    # The shared frontend is extracted from the verified amd64 artifact.
    prepared = tuple(dict.fromkeys(('amd64',) + requested))
    proxies = json.loads((ROOT / 'app/proxy/vendor/manifest.json').read_text())['binaries']
    subprocess.run(['npm', 'ci', '--prefix', 'app/build-tools', '--ignore-scripts', '--no-audit', '--no-fund'], cwd=ROOT, check=True)
    for target_arch in prepared:
        name = 'patch-manifest.json' if target_arch == 'amd64' else f'patch-manifest-{target_arch}.json'
        manifest = json.loads((ROOT / 'app' / name).read_text())
        with tempfile.TemporaryDirectory() as folder:
            original = Path(folder) / 'upstream'
            original.write_bytes(fetch(UPSTREAM.format(target_arch), manifest['input_sha256']))
            subprocess.run(['python3', '-B', 'app/patch-release.py', '--arch', target_arch, '--original', str(original)], cwd=ROOT, check=True)
        proxy_arch = 'linux-amd64-compatible' if target_arch == 'amd64' else 'linux-' + target_arch
        proxy = next(x for x in proxies if x['arch'] == proxy_arch)
        binary = gzip.decompress(fetch(proxy['url'], proxy['archive_sha256']))
        if hashlib.sha256(binary).hexdigest() != proxy['sha256']:
            raise SystemExit('Mihomo binary failed SHA256 verification')
        target = ROOT / 'app/proxy/vendor' / ('mihomo-' + proxy_arch)
        temp = target.with_suffix('.tmp')
        temp.write_bytes(binary)
        temp.chmod(0o755)
        temp.replace(target)
    subprocess.run(['python3', '-B', 'app/scheduler/build-assets.py'], cwd=ROOT, check=True)
    subprocess.run(['python3', '-B', 'app/verify-build.py', '--arch', arch], cwd=ROOT, check=True)


if __name__ == '__main__':
    main()
