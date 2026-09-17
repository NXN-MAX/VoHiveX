#!/usr/bin/env python3
"""Create self-contained packages from allowlisted program inputs only."""
import argparse
import hashlib
from pathlib import Path
import runpy
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def files_in(folder, suffixes=None):
    return [p.relative_to(ROOT).as_posix() for p in sorted((ROOT / folder).rglob('*'))
            if p.is_file() and not p.is_symlink()
            and not any(part.startswith('.') or part in ('__pycache__', 'node_modules') for part in p.relative_to(ROOT).parts)
            and (suffixes is None or p.suffix in suffixes or p.name in ('LICENSE', 'NOTICE'))]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output-dir', type=Path, default=ROOT / 'dist')
    args = parser.parse_args()
    runpy.run_path(str(ROOT / 'app/verify-build.py'))['verify'](ROOT)
    base = [
        '.dockerignore', '.env.example', 'LICENSE', 'Dockerfile', 'Dockerfile.vohivex',
        'docker-compose.yml', 'docker-compose.single.yml', 'DEPLOY.md', 'go.mod', 'go.sum',
        'release/vohivex-amd64', 'release/patch-result.json',
        'app/driver-lib.sh', 'app/driver.sh', 'app/single-start.sh', 'app/verify-build.py',
        'app/proxy/vendor/mihomo-linux-amd64-compatible',
        'app/proxy/vendor/manifest.json', 'app/proxy/vendor/LICENSE.mihomo',
        'app/proxy/vendor/LICENSE.jsQR', 'app/proxy/THIRD-PARTY.md',
    ] + [f'release/{name}' for name in ('vohivex-arm64', 'vohivex-armv7', 'patch-result-arm64.json', 'patch-result-armv7.json')] + [f'app/proxy/vendor/mihomo-linux-{arch}' for arch in ('arm64','armv7')] + files_in('app/scheduler/assets') + files_in('cmd/vohivex-gateway', {'.go'})
    extra = ['README.md', 'RELEASE_NOTES.md', 'DOCKERHUB.md', '.gitignore', 'app/README.md',
             'app/proxy/README.md', 'app/patch-manifest.json', 'app/patch-release.py',
             'app/package-single.py', 'app/prepare-build.py', 'app/patch-manifest-arm64.json', 'app/patch-manifest-armv7.json', 'app/build-tools/package.json', 'app/build-tools/package-lock.json']
    extra += [p.relative_to(ROOT).as_posix() for p in sorted((ROOT / 'app/scheduler').iterdir())
              if p.is_file() and p.suffix in ('.py', '.js', '.css', '.md')]
    extra += files_in('app/frontend', {'.py', '.cjs', '.js', '.css', '.json', '.svg', '.md'})
    extra += files_in('app/branding', {'.py', '.png', '.ico', '.ttf', '.txt', '.json', '.md'})
    extra += files_in('app/docs', {'.html', '.js', '.css', '.json', '.md', '.txt', '.png', '.map'})
    extra += ['docs/images/' + name for name in ('vohivex-banner.png', 'dashboard.png', 'proxy.png', 'sms.png', 'tasks.png')]
    packages = [('VoHiveX-deploy.zip', sorted(set(base))), ('VoHiveX-single.zip', sorted(set(base + extra)))]
    # Validate all inputs before creating or replacing either archive.
    for name in sorted(set(base + extra)):
        path = ROOT / name
        if not path.is_file() or path.is_symlink():
            raise SystemExit(f'Missing or symlinked package input: {name}')
    output = args.output_dir.expanduser().resolve()
    output.mkdir(parents=True, exist_ok=True)
    for filename, entries in packages:
        target = output / filename
        with tempfile.NamedTemporaryFile(dir=output, suffix='.zip', delete=False) as tmp:
            temp = Path(tmp.name)
        try:
            hashes = []
            with zipfile.ZipFile(temp, 'w', zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
                for name in entries:
                    data = (ROOT / name).read_bytes()
                    archive.write(ROOT / name, 'VoHiveX/' + name)
                    hashes.append(hashlib.sha256(data).hexdigest() + '  ' + name)
                archive.writestr('VoHiveX/SHA256SUMS', '\n'.join(hashes) + '\n')
            temp.replace(target)
        finally:
            temp.unlink(missing_ok=True)
        print(f'{target}: {len(entries)} files, {target.stat().st_size} bytes, SHA256 {hashlib.sha256(target.read_bytes()).hexdigest()}')


if __name__ == '__main__':
    main()
