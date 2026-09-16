#!/usr/bin/env python3
"""Run inside each CI image, without host devices or real user data."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid
import yaml


def request(port, path, body=None, token=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(f'http://127.0.0.1:{port}{path}',
                                 data=json.dumps(body).encode() if body is not None else None, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def main():
    spec = importlib.util.spec_from_file_location('init_config', '/opt/vohivex/init-config.py')
    initializer = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(initializer)
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        (root / 'data').mkdir()
        (root / 'logs').mkdir()
        config = root / 'config.yaml'
        initializer.prepare(config)
        initial = config.read_bytes()
        assert yaml.safe_load(initial)['web'] == {'username': 'admin', 'password': 'admin'}
        assert config.stat().st_mode & 0o777 == 0o600
        initializer.prepare(config)
        assert config.read_bytes() == initial, 'Existing config was overwritten'
        existing = root / 'existing.yaml'
        password = uuid.uuid4().hex
        existing.write_text('server:\n  port: ":7575" # retained comment\nweb:\n  username: "existing-user"\n  password: "' + password + '"\ndevices: []\n')
        previous = yaml.safe_load(existing.read_text())
        initializer.prepare(existing)
        migrated = yaml.safe_load(existing.read_text())
        assert migrated['web'] == previous['web'] and migrated['devices'] == previous['devices']
        assert migrated['server']['port'] == '127.0.0.1:7576'
        assert '# retained comment' in existing.read_text()
        initializer.prepare(existing)
        assert yaml.safe_load(existing.read_text()) == migrated
        env = dict(os.environ, CONFIG_PATH=str(config), SCHEDULER_DB=str(root / 'tasks.sqlite3'),
                   PROXY_DATA=str(root / 'proxy'))
        processes = []
        with (root / 'startup.log').open('w+') as log:
            try:
                processes.append(subprocess.Popen(['/app/vohive', '-c', str(config)], cwd=root, env=env, stdout=log, stderr=log))
                for _ in range(90):
                    try:
                        status, body = request(7576, '/api/auth/login', {'username': 'admin', 'password': 'admin'})
                        if status == 200:
                            auth = json.loads(body)
                            token = auth.get('token') or auth.get('data', {}).get('token')
                            assert token
                            break
                    except (OSError, urllib.error.URLError):
                        pass
                    assert processes[0].poll() is None, 'Core exited before login'
                    time.sleep(1)
                else:
                    raise AssertionError('Core login timed out')
                # Exercise the actual architecture-specific Go stubs directly,
                # not a gateway response that might conceal a broken binary patch.
                for _ in range(12):
                    for path, payload in [('/api/system/info', None), ('/api/system/update/check', None),
                                          ('/api/system/update/apply', {})]:
                        assert request(7576, path, payload, token)[0] == 410, path
                processes.append(subprocess.Popen(['python3', '-B', '/opt/vohivex/scheduler/server.py'], cwd=root, env=env, stdout=log, stderr=log))
                for _ in range(60):
                    try:
                        status, body = request(7575, '/')
                        if status == 200:
                            assert b'VoHiveX' in body
                            break
                    except (OSError, urllib.error.URLError):
                        pass
                    assert all(p.poll() is None for p in processes), 'Service exited before gateway startup'
                    time.sleep(1)
                else:
                    raise AssertionError('Gateway startup timed out')
                status, body = request(7575, '/api/auth/login', {'username': 'admin', 'password': 'admin'})
                assert status == 200 and json.loads(body).get('token'), 'Default gateway login failed'
                assert request(7575, '/api/devices', token=token)[0] == 200
                assert request(7575, '/api/schedules', token=token)[0] == 200
                assert request(7575, '/api/docs')[0] == 200
                assert request(7575, '/api/system/update/check', token=token)[0] == 410
                version = subprocess.check_output(['/opt/vohivex/proxy/mihomo', '-v'], timeout=15).decode()
                assert 'v1.19.31' in version
                print('PASS: CPU binaries, admin/admin login, port 7575, preserved config, disabled update handlers, scheduler and API docs')
            except Exception:
                log.flush()
                log.seek(0)
                print(log.read()[-6000:])
                raise
            finally:
                for process in reversed(processes):
                    process.terminate()
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()


if __name__ == '__main__':
    main()
