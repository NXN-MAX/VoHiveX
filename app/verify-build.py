#!/usr/bin/env python3
"""Read-only validation of the prepared multi-architecture Docker build context."""
import argparse
import ast
import fnmatch
import hashlib
import json
from pathlib import Path
import shlex

ROOT = Path(__file__).resolve().parents[1]


def verify(root=ROOT, arch="all"):
    errors = []
    checked = set()
    def require(name):
        path = root / name
        if not path.is_file() or path.is_symlink():
            errors.append(f'Missing or symlinked build input: {name}')
            return None
        checked.add(name)
        return path

    for name in ('Dockerfile', 'Dockerfile.vohivex', 'docker-compose.yml',
                 'docker-compose.single.yml', '.dockerignore'):
        require(name)
    for left, right in [('Dockerfile', 'Dockerfile.vohivex'),
                        ('docker-compose.yml', 'docker-compose.single.yml')]:
        if (root / left).is_file() and (root / right).is_file():
            if (root / left).read_bytes() != (root / right).read_bytes():
                errors.append(f'Build entrypoints differ: {left}, {right}')

    # Current ignore file uses *, explicit negations, and directory negations.
    patterns = (root / '.dockerignore').read_text().splitlines() if (root / '.dockerignore').is_file() else []
    def included(name):
        allowed = True
        for pattern in patterns:
            pattern = pattern.strip()
            if not pattern or pattern.startswith('#'):
                continue
            negative = pattern.startswith('!')
            pattern = pattern.lstrip('!').rstrip('/')
            if fnmatch.fnmatchcase(name, pattern) or name.startswith(pattern + '/'):
                allowed = negative
        return allowed

    dockerfile = root / 'Dockerfile.vohivex'
    if dockerfile.is_file():
        for line in dockerfile.read_text().splitlines():
            if not line.startswith('COPY ') or '--from=' in line:
                continue
            for source in shlex.split(line)[1:-1]:
                path = root / source
                files = sorted(root.glob(source)) if '*' in source else sorted(p for p in path.rglob('*') if p.is_file()) if path.is_dir() else [path]
                if not files:
                    errors.append(f'Empty Docker COPY source: {source}')
                for path in files:
                    name = path.relative_to(root).as_posix()
                    if require(name) and not included(name):
                        errors.append(f'Docker COPY input excluded by .dockerignore: {name}')

    for name in ('app/scheduler/assets/index.html', 'app/scheduler/assets/build-info.json',
                 'app/scheduler/assets/api/docs/index.html'):
        require(name)
    architectures = ('amd64', 'arm64', 'armv7') if arch == 'all' else (arch,)
    vendor = json.loads((root / 'app/proxy/vendor/manifest.json').read_text())
    for target in architectures:
        report_name = 'patch-result.json' if target == 'amd64' else f'patch-result-{target}.json'
        report_path = require('release/' + report_name)
        if not report_path:
            continue
        report = json.loads(report_path.read_text())
        proxy_arch = 'linux-amd64-compatible' if target == 'amd64' else 'linux-' + target
        proxy = next(b for b in vendor['binaries'] if b['arch'] == proxy_arch)
        elf_class, machine = {'amd64': (2, 62), 'arm64': (2, 183), 'armv7': (1, 40)}[target]
        for binary_name, expected in [(f'release/vohive-dji-{target}', report['patched_sha256']),
                                       (f'app/proxy/vendor/mihomo-{proxy_arch}', proxy['sha256'])]:
            binary = require(binary_name)
            if not binary:
                continue
            data = binary.read_bytes()
            if hashlib.sha256(data).hexdigest() != expected:
                errors.append(f'Binary checksum mismatch: {binary_name}')
            if data[:4] != b'\x7fELF' or data[4] != elf_class or int.from_bytes(data[18:20], 'little') != machine:
                errors.append(f'Wrong ELF architecture: {binary_name}')
    for name in sorted(checked):
        if name.endswith('.py'):
            ast.parse((root / name).read_text(), filename=name)
    if errors:
        raise SystemExit('\n'.join(errors))
    print(f'Build inputs OK: {len(checked)} files; binary hashes, ELF architectures, Docker COPY and Python syntax verified.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', choices=('all', 'amd64', 'arm64', 'armv7'), default='all')
    verify(arch=parser.parse_args().arch)
