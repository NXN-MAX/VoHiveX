"""Durable SMS scheduler. No catch-up bursts and no automatic send retries."""
import contextlib
import json
import math
import os
import re
import sqlite3
import threading
import time
import uuid


class Invalid(ValueError):
    pass


class Conflict(Invalid):
    pass


def validate(data, now):
    if not isinstance(data, dict):
        raise Invalid('任务格式错误')
    result = {}
    for key, label, limit in [('name', '任务名称', 80), ('device_id', '发信设备', 128),
                              ('phone', '收信号码', 24), ('message', '短信内容', 2000)]:
        value = data.get(key, '')
        if not isinstance(value, str) or not value.strip() or len(value) > limit:
            raise Invalid(f'{label}不能为空，且不能超过 {limit} 个字符')
        result[key] = value if key == 'message' else value.strip()
    if not re.fullmatch(r'\+?[0-9]{3,20}', result['phone']):
        raise Invalid('收信号码仅支持一个号码：数字及可选的 + 国家区号')
    if data.get('mode') not in ('once', 'interval'):
        raise Invalid('请选择单次或间隔重复')
    result['mode'] = data['mode']
    first = data.get('first_run')
    if isinstance(first, bool) or not isinstance(first, (int, float)) or not math.isfinite(first):
        raise Invalid('请设置有效的首次执行时间')
    if first <= now:
        raise Invalid('首次执行时间必须晚于当前时间')
    if first > now + 366 * 86400 * 10:
        raise Invalid('首次执行时间不能超过十年')
    result['first_run'] = int(first)
    interval = data.get('interval_seconds', 0)
    if isinstance(interval, bool) or not isinstance(interval, int):
        raise Invalid('重复间隔必须是整数秒')
    if result['mode'] == 'interval' and not 1 <= interval <= 366 * 86400 * 10:
        raise Invalid('重复间隔至少 1 秒，最多十年')
    result['interval_seconds'] = interval if result['mode'] == 'interval' else 0
    return result


class Store:
    def __init__(self, path, clock=time.time):
        self.path, self.clock = str(path), clock
        os.makedirs(os.path.dirname(os.path.abspath(self.path)), exist_ok=True)
        with self.db() as db:
            db.executescript('''
                PRAGMA journal_mode=WAL;
                CREATE TABLE IF NOT EXISTS tasks (
                    id TEXT PRIMARY KEY, name TEXT NOT NULL, device_id TEXT NOT NULL,
                    phone TEXT NOT NULL, message TEXT NOT NULL, mode TEXT NOT NULL,
                    first_run INTEGER NOT NULL, interval_seconds INTEGER NOT NULL,
                    state TEXT NOT NULL DEFAULT 'paused', next_run INTEGER,
                    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
                    last_run INTEGER, last_result TEXT NOT NULL DEFAULT '',
                    run_count INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1
                );
                CREATE TABLE IF NOT EXISTS runs (
                    id TEXT PRIMARY KEY, task_id TEXT NOT NULL, scheduled_for INTEGER NOT NULL,
                    started_at INTEGER NOT NULL, finished_at INTEGER,
                    status TEXT NOT NULL, detail TEXT NOT NULL DEFAULT '',
                    UNIQUE(task_id, scheduled_for)
                );
                CREATE TABLE IF NOT EXISTS notifications (
                    run_id TEXT PRIMARY KEY, task_id TEXT NOT NULL,
                    event TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'pending',
                    created_at INTEGER NOT NULL, detail TEXT NOT NULL DEFAULT ''
                );
                CREATE TABLE IF NOT EXISTS delivery_checks (
                    run_id TEXT PRIMARY KEY, event TEXT NOT NULL,
                    next_check INTEGER NOT NULL, deadline INTEGER NOT NULL,
                    state TEXT NOT NULL DEFAULT 'waiting', detail TEXT NOT NULL DEFAULT ''
                );
            ''')
        os.chmod(self.path, 0o600)

    @contextlib.contextmanager
    def db(self):
        db = sqlite3.connect(self.path, timeout=10)
        db.row_factory = sqlite3.Row
        try:
            with db:
                yield db
        finally:
            db.close()

    def list(self):
        with self.db() as db:
            return [dict(r) for r in db.execute('SELECT * FROM tasks ORDER BY created_at DESC,id')]

    def create(self, payload):
        now = int(self.clock())
        data = validate(payload, now)
        data.update(id=uuid.uuid4().hex, created_at=now, updated_at=now)
        with self.db() as db:
            if db.execute('SELECT count(*) FROM tasks').fetchone()[0] >= 1000:
                raise Invalid('最多保存 1000 个任务')
            cols = ','.join(data)
            db.execute(f'INSERT INTO tasks ({cols}) VALUES ({",".join("?" for _ in data)})', list(data.values()))
        return data['id']

    def change(self, task_id, action, payload):
        now = int(self.clock())
        with self.db() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute('SELECT * FROM tasks WHERE id=?', (task_id,)).fetchone()
            if not row:
                raise KeyError(task_id)
            expected = payload.get('version')
            if type(expected) is not int or expected != row['version']:
                raise Conflict('任务已被修改，请刷新后再操作')
            busy = db.execute("SELECT 1 FROM runs WHERE task_id=? AND status='sending'", (task_id,)).fetchone()
            if busy and action != 'pause':
                raise Conflict('该任务正在发送，请等待本次执行结束；可先暂停后续执行')
            if action == 'delete':
                db.execute('DELETE FROM runs WHERE task_id=?', (task_id,))
                db.execute('DELETE FROM tasks WHERE id=?', (task_id,))
                return
            if action == 'edit':
                data = validate(payload, now)
                data.update(state='paused', next_run=None, last_result='已修改，等待开始')
            elif action == 'start':
                if row['state'] == 'active':
                    raise Conflict('任务已经开始')
                if row['mode'] == 'once' and row['first_run'] <= now:
                    raise Invalid('单次执行时间已过，请修改时间后再开始')
                due = row['first_run']
                if due <= now:
                    due += ((now - due) // row['interval_seconds'] + 1) * row['interval_seconds']
                data = dict(state='active', next_run=due, last_result='等待执行')
            elif action == 'pause':
                data = dict(state='paused', next_run=None)
            else:
                raise Invalid('不支持的操作')
            data.update(updated_at=now, version=row['version'] + 1)
            db.execute('UPDATE tasks SET ' + ','.join(k+'=?' for k in data) + ' WHERE id=?', [*data.values(), task_id])

    def history(self, task_id):
        with self.db() as db:
            return [dict(r) for r in db.execute('SELECT * FROM runs WHERE task_id=? ORDER BY started_at DESC LIMIT 20', (task_id,))]

    @staticmethod
    def future(row, now):
        base = row['first_run']
        return base + max(0, (int(now) - base) // row['interval_seconds'] + 1) * row['interval_seconds']

    def recover(self):
        """Unknown in-flight results are paused. Downtime never causes catch-up SMS."""
        now = int(self.clock())
        with self.db() as db:
            db.execute('BEGIN IMMEDIATE')
            db.execute("UPDATE tasks SET state='paused',next_run=NULL,last_result='上次发送结果未知，已暂停，请核对短信记录',version=version+1 WHERE id IN (SELECT task_id FROM runs WHERE status='sending')")
            db.execute("UPDATE runs SET status='unknown',finished_at=?,detail='进程中断，未自动重发' WHERE status='sending'", (now,))
            for row in db.execute("SELECT * FROM tasks WHERE state='active' AND next_run<=?", (now,)).fetchall():
                if row['mode'] == 'interval':
                    db.execute("UPDATE tasks SET next_run=?,last_result='已跳过停机期间的执行时间',version=version+1 WHERE id=?", (self.future(row, now), row['id']))
                else:
                    db.execute("UPDATE tasks SET state='paused',next_run=NULL,last_result='执行时间在停机期间已过，请修改后开始',version=version+1 WHERE id=?", (row['id'],))

    def claim(self):
        now = int(self.clock())
        with self.db() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute("SELECT * FROM tasks WHERE state='active' AND next_run<=? ORDER BY next_run,id LIMIT 1", (now,)).fetchone()
            if not row:
                return None
            run_id = uuid.uuid4().hex
            if now - row['next_run'] > 60:
                db.execute('INSERT INTO runs VALUES (?,?,?,?,?,?,?)', (run_id,row['id'],row['next_run'],now,now,'skipped','超过执行时间 60 秒，未补发'))
                state = 'active' if row['mode'] == 'interval' else 'paused'
                next_run = self.future(row, now) if state == 'active' else None
                db.execute('UPDATE tasks SET state=?,next_run=?,last_result=?,version=version+1 WHERE id=?', (state,next_run,'已跳过错过的执行时间',row['id']))
                return None
            # Commit before calling SMS API. Never retry an ambiguous network result.
            db.execute('INSERT INTO runs VALUES (?,?,?,?,NULL,?,?)', (run_id,row['id'],row['next_run'],now,'sending',''))
            db.execute("UPDATE tasks SET next_run=NULL,last_run=?,last_result='发送中',version=version+1 WHERE id=?", (now,row['id']))
            return dict(row), run_id

    def finish(self, task, run_id, status, detail):
        now = int(self.clock())
        with self.db() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute('SELECT * FROM tasks WHERE id=?', (task['id'],)).fetchone()
            if not row:
                return
            run = db.execute("SELECT * FROM runs WHERE id=? AND status='sending'", (run_id,)).fetchone()
            if not run:
                return
            state, due = row['state'], None
            if status != 'success':
                state = 'paused'
            elif state == 'active':
                if row['mode'] == 'once':
                    state = 'completed'
                else:
                    due = self.future(row, now)
            db.execute('UPDATE runs SET status=?,detail=?,finished_at=? WHERE id=?', (status,detail,now,run_id))
            event = {key: task.get(key, '') for key in ('name','device_id','device_name','phone','message')}
            event.update(task_id=task['id'], run_id=run_id, status=status, detail=detail,
                         started_at=run['started_at'], finished_at=now)
            event['send_result'] = task.get('send_result') or {}
            db.execute('INSERT OR IGNORE INTO notifications (run_id,task_id,event,created_at) VALUES (?,?,?,?)',
                       (run_id,task['id'],json.dumps(event,ensure_ascii=False),now))
            db.execute('INSERT OR IGNORE INTO delivery_checks (run_id,event,next_check,deadline) VALUES (?,?,?,?)',
                       (run_id,json.dumps(event,ensure_ascii=False),now,now+86400))
            db.execute('UPDATE tasks SET state=?,next_run=?,last_result=?,run_count=run_count+?,updated_at=?,version=version+1 WHERE id=?', (state,due,detail,int(status=='success'),now,task['id']))
            db.execute('DELETE FROM runs WHERE task_id=? AND id NOT IN (SELECT id FROM runs WHERE task_id=? ORDER BY started_at DESC LIMIT 100)', (task['id'],task['id']))

    def recover_notifications(self):
        # A process may have stopped after the remote service accepted a push.
        # Preserve its uncertain outcome, rather than sending duplicates.
        with self.db() as db:
            db.execute("UPDATE notifications SET state='unknown',detail='推送进程中断，结果未知，未自动重发' WHERE state='sending'")

    def claim_notification(self):
        with self.db() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute("SELECT * FROM notifications WHERE state='pending' ORDER BY created_at,rowid LIMIT 1").fetchone()
            if not row:
                return None
            db.execute("UPDATE notifications SET state='sending' WHERE run_id=?", (row['run_id'],))
            event = json.loads(row['event'])
            event['_notification_id'] = row['run_id']
            return event

    def finish_notification(self, run_id, state, detail):
        with self.db() as db:
            db.execute('UPDATE notifications SET state=?,detail=? WHERE run_id=?', (state,detail,run_id))
            db.execute("DELETE FROM notifications WHERE state NOT IN ('pending','sending') AND rowid NOT IN (SELECT rowid FROM notifications ORDER BY created_at DESC,rowid DESC LIMIT 1000)")

    def next_delivery(self):
        with self.db() as db:
            row = db.execute("SELECT * FROM delivery_checks WHERE state='waiting' AND next_check<=? ORDER BY next_check,rowid LIMIT 1", (int(self.clock()),)).fetchone()
            return dict(row) if row else None

    def defer_delivery(self, run_id):
        with self.db() as db:
            db.execute("UPDATE delivery_checks SET next_check=? WHERE run_id=? AND state='waiting'", (int(self.clock())+15,run_id))

    def finish_delivery(self, run_id, status, detail):
        now = int(self.clock())
        with self.db() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute("SELECT * FROM delivery_checks WHERE run_id=? AND state='waiting'", (run_id,)).fetchone()
            if not row:
                return
            event = json.loads(row['event'])
            event.update(phase='delivery', status=status, detail=detail, finished_at=now,
                         execution_finished_at=event['finished_at'])
            db.execute('INSERT OR IGNORE INTO notifications (run_id,task_id,event,created_at) VALUES (?,?,?,?)',
                       (run_id+':delivery',event['task_id'],json.dumps(event,ensure_ascii=False),now))
            db.execute('UPDATE delivery_checks SET state=?,detail=? WHERE run_id=?', (status,detail,run_id))
            db.execute("DELETE FROM delivery_checks WHERE state!='waiting' AND rowid NOT IN (SELECT rowid FROM delivery_checks ORDER BY rowid DESC LIMIT 1000)")


class Worker:
    def __init__(self, store, sender):
        self.store, self.sender = store, sender
        self.stop = threading.Event()

    def tick(self):
        claimed = self.store.claim()
        if not claimed:
            return
        task, run_id = claimed
        try:
            status, detail = self.sender(task)
        except Exception:
            status, detail = 'unknown', '发送结果未知，已暂停，请核对短信记录'
        self.store.finish(task, run_id, status, detail)

    def run(self):
        self.store.recover()
        while not self.stop.is_set():
            try:
                self.tick()
            except Exception as exc:
                print('Scheduler tick failed:', type(exc).__name__, flush=True)
            self.stop.wait(0.25)
