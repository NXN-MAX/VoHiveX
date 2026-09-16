"""Authenticated username updates; passwords and device settings are unchanged."""
import json
import os
from pathlib import Path
import re
import tempfile
import threading
import yaml

LOCK = threading.Lock()


def username_from(path):
    value = yaml.safe_load(Path(path).read_text())
    return value['web']['username']


def rename_username(path, username, password, authenticate):
    if not isinstance(username, str) or not re.fullmatch(r'[A-Za-z0-9_.@-]{3,64}', username):
        raise ValueError('用户名需为 3–64 位字母、数字或 _ . @ -')
    if not isinstance(password, str) or not password or len(password) > 1024:
        raise ValueError('请填写当前密码')
    path = Path(path)
    with LOCK:
        original = path.read_text()
        value = yaml.safe_load(original)
        current = value['web']['username']
        if not authenticate(current, password):
            raise PermissionError('当前密码不正确')
        if username == current:
            raise ValueError('新用户名与当前用户名相同')
        # Replace one scalar only, preserving passwords, comments and modem settings.
        root = yaml.compose(original)
        web = next(v for k, v in root.value if k.value == 'web')
        scalar = next(v for k, v in web.value if k.value == 'username')
        updated = original[:scalar.start_mark.index] + json.dumps(username) + original[scalar.end_mark.index:]
        expected = dict(value, web=dict(value['web'], username=username))
        if yaml.safe_load(updated) != expected:
            raise ValueError('配置格式不支持安全更新')
        if path.read_text() != original:
            raise ValueError('配置已变更，请刷新后重试')
        # Keep one restricted backup for recovery and replace on the same filesystem.
        backup = path.with_name(path.name + '.before-username')
        for destination, content in [(backup, original), (path, updated)]:
            fd, name = tempfile.mkstemp(prefix='.username-', dir=path.parent)
            try:
                with os.fdopen(fd, 'w') as out:
                    out.write(content)
                    out.flush()
                    os.fsync(out.fileno())
                os.replace(name, destination)
            finally:
                if os.path.exists(name):
                    os.unlink(name)
        return username
