#!/usr/bin/env python3
"""Build the Vue 3 frontend and assemble the runtime web asset directory."""
from datetime import datetime, timezone
import argparse
import json
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[2]
WEB = ROOT / 'web'
OUTPUT = ROOT / 'app/scheduler/assets'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--skip-install', action='store_true', help='Use the existing web/node_modules directory')
    args = parser.parse_args()
    if not args.skip_install:
        subprocess.run(['npm', 'ci', '--prefix', 'web', '--ignore-scripts', '--no-audit', '--no-fund'], cwd=ROOT, check=True)
    subprocess.run(['npm', 'run', 'build', '--prefix', 'web'], cwd=ROOT, check=True)
    if OUTPUT.exists():
        shutil.rmtree(OUTPUT)
    shutil.copytree(WEB / 'dist', OUTPUT)
    docs = OUTPUT / 'api/docs'
    docs.mkdir(parents=True)
    shutil.copyfile(ROOT / 'app/docs/index.html', docs / 'index.html')
    shutil.copytree(ROOT / 'app/docs/vendor/swagger-ui-5.32.15', docs / 'assets/5.32.15')
    (OUTPUT / 'build-info.json').write_text(json.dumps({
        'frontend': 'Vue 3 + Vite + Pinia + Antdv Next',
        'build_time': datetime.now(timezone.utc).isoformat(),
    }, ensure_ascii=False, indent=2) + '\n')
    print(f'Prepared {sum(path.is_file() for path in OUTPUT.rglob("*"))} frontend assets')


if __name__ == '__main__':
    main()
