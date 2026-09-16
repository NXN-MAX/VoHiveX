#!/usr/bin/env python3
"""Initialize new installations and keep the core listener behind the gateway."""
import json
import os
from pathlib import Path
import tempfile
import yaml

CORE_LISTEN = '127.0.0.1:7576'


def prepare(path):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    if not path.exists():
        # Exclusive creation never replaces an existing installation's credentials.
        content = 'server:\n  port: "127.0.0.1:7576"\n\nweb:\n  username: "admin"\n  password: "admin"\n\ndevices: []\n'
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as out:
            out.write(content)
        return
    original = path.read_text()
    config = yaml.safe_load(original)
    if not isinstance(config, dict) or not isinstance(config.get('web'), dict):
        raise ValueError('Existing configuration is invalid; refusing to replace it')
    if config.get('server', {}).get('port') == CORE_LISTEN:
        return
    # Change only the internal listener scalar. Preserve credentials, comments,
    # modem settings, proxy settings, and all other existing configuration bytes.
    root = yaml.compose(original)
    server = next((v for k, v in root.value if k.value == 'server'), None)
    port = next((v for k, v in server.value if k.value == 'port'), None) if isinstance(server, yaml.MappingNode) else None
    if not isinstance(port, yaml.ScalarNode):
        raise ValueError('Existing config needs a server.port scalar; refusing automatic migration')
    updated = original[:port.start_mark.index] + json.dumps(CORE_LISTEN) + original[port.end_mark.index:]
    expected = dict(config, server=dict(config['server'], port=CORE_LISTEN))
    if yaml.safe_load(updated) != expected or path.read_text() != original:
        raise ValueError('Configuration changed during migration')
    fd, temporary = tempfile.mkstemp(prefix='.listener-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as out:
            out.write(updated)
            out.flush()
            os.fsync(out.fileno())
        os.replace(temporary, path)
    finally:
        Path(temporary).unlink(missing_ok=True)


if __name__ == '__main__':
    os.umask(0o077)
    prepare(os.environ.get('CONFIG_PATH', '/app/config/config.yaml'))
