"""Local SMS archive used by the browser import/export tools.

Imported records live beside the existing scheduler database in /app/data.
They are deliberately separate from the upstream modem database so importing
an archive can never trigger a modem write or an SMS send.
"""
from datetime import datetime, timezone
import hashlib
from pathlib import Path
import sqlite3
import threading


def _timestamp_ms(value):
    text = str(value or '').strip()
    if not text:
        return int(datetime.now(timezone.utc).timestamp() * 1000)
    try:
        parsed = datetime.fromisoformat(text.replace('Z', '+00:00'))
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return int(parsed.timestamp() * 1000)
    except ValueError:
        raise ValueError('短信时间格式无效') from None


class SmsArchive:
    def __init__(self, path):
        self.path = str(path)
        self.lock = threading.RLock()
        if self.path != ':memory:' and not self.path.startswith('file:'):
            Path(self.path).expanduser().parent.mkdir(parents=True, exist_ok=True)
        with self._connect() as db:
            db.execute('''CREATE TABLE IF NOT EXISTS imported_sms (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                device_id TEXT NOT NULL,
                device_name TEXT NOT NULL DEFAULT '',
                local_phone TEXT NOT NULL DEFAULT '',
                imsi TEXT NOT NULL,
                peer TEXT NOT NULL,
                message_type INTEGER NOT NULL,
                content TEXT NOT NULL,
                timestamp TEXT NOT NULL,
                timestamp_ms INTEGER NOT NULL,
                sender TEXT NOT NULL DEFAULT '',
                status INTEGER,
                source_message_id TEXT NOT NULL DEFAULT '',
                signature TEXT NOT NULL UNIQUE,
                imported_at INTEGER NOT NULL
            )''')
            db.execute('''CREATE INDEX IF NOT EXISTS imported_sms_thread
                          ON imported_sms(device_id, imsi, peer, timestamp_ms, id)''')

    def _connect(self):
        db = sqlite3.connect(self.path, timeout=10)
        db.row_factory = sqlite3.Row
        db.execute('PRAGMA journal_mode=WAL')
        db.execute('PRAGMA busy_timeout=10000')
        return db

    def import_messages(self, device, rows):
        device_id = str(device.get('id') or '').strip()
        if not device_id:
            raise ValueError('请选择导入设备')
        device_name = str(device.get('name') or device_id).strip()[:200]
        modem = device.get('modem') if isinstance(device.get('modem'), dict) else {}
        default_imsi = str(device.get('imsi') or modem.get('imsi') or ('archive:' + device_id)).strip()[:128]
        local_phone = str(device.get('local_phone') or modem.get('msisdn') or modem.get('phone_number') or '').strip()[:80]
        now = int(datetime.now(timezone.utc).timestamp())
        inserted = 0
        with self.lock, self._connect() as db:
            for row in rows:
                peer = str(row.get('peer') or '').strip()
                content = str(row.get('content') or '')
                if not peer or len(peer) > 200 or not content or len(content) > 20000:
                    raise ValueError('短信联系人或内容无效')
                message_type = int(row.get('type') or 0)
                if message_type not in (1, 2):
                    raise ValueError('短信方向无效')
                timestamp = str(row.get('timestamp') or '').strip()
                stamp = _timestamp_ms(timestamp)
                if not timestamp:
                    timestamp = datetime.fromtimestamp(stamp / 1000, timezone.utc).isoformat()
                imsi = str(row.get('imsi') or default_imsi).strip()[:128] or default_imsi
                sender = str(row.get('sender') or '').strip()[:200]
                source_id = str(row.get('message_id') or '').strip()[:200]
                status = row.get('status')
                status = int(status) if status not in (None, '') else None
                if status is not None and status not in (0, 1, 2, 3):
                    status = None
                material = '\0'.join((device_id, imsi, peer, str(message_type), timestamp,
                                      content, sender, source_id)).encode('utf-8')
                signature = hashlib.sha256(material).hexdigest()
                cursor = db.execute('''INSERT OR IGNORE INTO imported_sms
                    (device_id, device_name, local_phone, imsi, peer, message_type,
                     content, timestamp, timestamp_ms, sender, status,
                     source_message_id, signature, imported_at)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)''',
                    (device_id, device_name, local_phone, imsi, peer, message_type,
                     content, timestamp, stamp, sender, status, source_id, signature, now))
                inserted += cursor.rowcount
        return inserted

    def contacts(self, device_id=None):
        query = 'SELECT * FROM imported_sms'
        args = []
        if device_id and device_id != 'all':
            query += ' WHERE device_id=?'
            args.append(device_id)
        query += ' ORDER BY timestamp_ms DESC, id DESC'
        result = []
        seen = set()
        with self.lock, self._connect() as db:
            for row in db.execute(query, args):
                key = (row['imsi'], row['peer'])
                if key in seen:
                    continue
                seen.add(key)
                result.append({
                    'imsi': row['imsi'], 'peer': row['peer'],
                    'device_id': row['device_id'], 'device_name': row['device_name'],
                    'local_phone': row['local_phone'],
                    'last_timestamp': row['timestamp'], 'last_sms_id': -row['id'],
                    'last_content': row['content'], 'imported': True,
                })
        return result

    def messages(self, peer, device_id=None, imsi=None, limit=200,
                 before_ts=None, before_id=None):
        clauses = ['peer=?']
        args = [peer]
        if device_id and device_id != 'all':
            clauses.append('device_id=?')
            args.append(device_id)
        elif imsi:
            clauses.append('imsi=?')
            args.append(imsi)
        if before_ts:
            cutoff = _timestamp_ms(before_ts)
            clauses.append('(timestamp_ms<? OR (timestamp_ms=? AND id>?))')
            args.extend((cutoff, cutoff, abs(int(before_id or 0))))
        limit = max(1, min(int(limit or 200), 500))
        query = ('SELECT * FROM imported_sms WHERE ' + ' AND '.join(clauses) +
                 ' ORDER BY timestamp_ms DESC, id DESC LIMIT ?')
        args.append(limit)
        with self.lock, self._connect() as db:
            rows = list(db.execute(query, args))
        rows.reverse()
        return [{
            'id': -row['id'], 'type': row['message_type'],
            'content': row['content'], 'timestamp': row['timestamp'],
            'sender': row['sender'], 'status': row['status'],
            'device_id': row['device_id'], 'device_name': row['device_name'],
            'local_phone': row['local_phone'], 'imsi': row['imsi'],
            'peer': row['peer'], 'imported': True,
        } for row in rows]

    def delete_message(self, message_id):
        archive_id = abs(int(message_id))
        with self.lock, self._connect() as db:
            row = db.execute('SELECT device_id, imsi, peer FROM imported_sms WHERE id=?',
                             (archive_id,)).fetchone()
            cursor = db.execute('DELETE FROM imported_sms WHERE id=?', (archive_id,))
            remaining = 0 if row is None else db.execute(
                'SELECT COUNT(*) FROM imported_sms WHERE device_id=? AND imsi=? AND peer=?',
                (row['device_id'], row['imsi'], row['peer'])).fetchone()[0]
        return {'deleted': cursor.rowcount, 'thread_empty': bool(row is not None and remaining == 0)}

    def delete_thread(self, peer, device_id=None, imsi=None):
        clauses = ['peer=?']
        args = [peer]
        if device_id and device_id != 'all':
            clauses.append('device_id=?')
            args.append(device_id)
        elif imsi:
            clauses.append('imsi=?')
            args.append(imsi)
        with self.lock, self._connect() as db:
            cursor = db.execute('DELETE FROM imported_sms WHERE ' + ' AND '.join(clauses), args)
        return cursor.rowcount
