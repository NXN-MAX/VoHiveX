#!/usr/bin/env python3
"""Reject runtime captures, subscriber numbers and credentials in Git's index.

This is a publication guard, not a complete secret detector. Review screenshots
and newly vendored source separately before publishing.
"""
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
BLOCKED_SUFFIXES = {'.log', '.sqlite', '.sqlite3', '.db', '.wav', '.pcm', '.raw', '.pcap', '.pcapng', '.har', '.pem', '.key', '.p12', '.pfx', '.jsonl'}
RULES = {
    'private key': rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',
    'personal host path': rb'/(?:Users|Volumes)/[^\s"\x27`<>]+',
    'subscriber phone number': rb'\+(?:86)?1[3-9][0-9]{9}(?![0-9])',
    'credential URL': rb'https?://[^\s"\x27<>]+(?:access_token|token|secret|key)=[A-Za-z0-9_-]{16,}',
}

def main():
    names = subprocess.check_output(['git', 'ls-files', '-z'], cwd=ROOT).decode().split('\0')
    problems = []
    for name in filter(None, names):
        p = Path(name)
        if p.suffix in BLOCKED_SUFFIXES or p.name in {'.env', '.DS_Store', 'network-debug-relay.json'} or any(x in p.parts for x in ('node_modules', 'recordings', 'private', 'runtime-data', 'network-debug-mihomo')):
            problems.append((name, 'runtime/private file')); continue
        data = subprocess.check_output(['git', 'show', ':' + name], cwd=ROOT)
        if b'\0' in data[:8192]:
            continue  # Binary screenshots/fonts need a separate visual review.
        for label, pattern in RULES.items():
            if re.search(pattern, data):
                problems.append((name, label))
    for name, label in problems:
        print(f'{name}: {label}', file=sys.stderr)  # Never echo the matched secret.
    print(f'Public-file check: {len(list(filter(None, names)))} files, {len(problems)} findings')
    return 1 if problems else 0

if __name__ == '__main__':
    raise SystemExit(main())
