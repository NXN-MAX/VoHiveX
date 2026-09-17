#!/usr/bin/env python3
"""Same-container gateway and authenticated scheduled-SMS API."""
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import platform
import re
import subprocess
from functools import lru_cache
from pathlib import Path
import select
import signal
import threading
import time
from datetime import datetime
from urllib.parse import urlsplit, unquote, quote, parse_qs
import yaml
from engine import Store, Worker, Invalid, Conflict
from egress_ip import EgressProbe, device_egress
from managed_proxy import Manager
from account import username_from, rename_username
from notifications import Dispatcher, NotificationWorker
from delivery import DeliveryWorker
from sms_archive import SmsArchive

UPSTREAM_HOST = os.environ.get('VOHIVE_UPSTREAM_HOST', '127.0.0.1')
UPSTREAM_PORT = int(os.environ.get('VOHIVE_UPSTREAM_PORT', '7576'))
ASSETS = Path(os.environ.get('SCHEDULER_ASSETS', '/opt/vohivex/scheduler/assets'))
CONFIG = Path(os.environ.get('CONFIG_PATH', '/app/config/config.yaml'))
STORE = None
PROXIES = None
ACCOUNT_RESTART = None
SMS_ARCHIVE = None
HOP = {'connection','keep-alive','proxy-authenticate','proxy-authorization','te','trailer','transfer-encoding','upgrade'}
BUILTIN_ID = 'vohive-mihomo'
BUILTIN_LOCK = threading.RLock()
EGRESS = EgressProbe()


def driver_version():
    if platform.system() != 'Linux':
        return 'option / qmi_wwan · 当前系统未加载'
    root = Path(os.environ.get('QMI_SYS_ROOT', '/host-sys')) / 'module'
    versions = []
    for name in ('option', 'qmi_wwan'):
        module = root / name
        if not module.is_dir():
            versions.append(f'{name} · 未加载')
            continue
        try:
            version = (module / 'version').read_text().strip()
        except OSError:
            version = ''
        versions.append(f'{name} · {version}' if version else f'{name} · 内核 {platform.release()}')
    return '\n'.join(versions)


@lru_cache(maxsize=4)
def proxy_version(binary):
    # -v only prints the installed executable version; it does not start a proxy.
    try:
        result = subprocess.run([binary, '-v'], capture_output=True, text=True, timeout=3, check=True)
        match = re.search(r'\bMihomo\s+(?:Meta\s+)?(v?\d[^\s]*)', result.stdout, re.I)
        if match:
            return 'Mihomo ' + match.group(1)
    except (OSError, subprocess.SubprocessError, UnicodeError):
        pass
    return '暂不可用'


def builtin_data(enabled=False):
    return {'id':BUILTIN_ID,'name':'订阅节点 · 内置代理','addr':PROXIES.endpoint,
            'username':'','password':'','enabled':enabled}


def proxy_rows(value):
    return value if isinstance(value,list) else value.get('proxies',[]) if isinstance(value,dict) else []


def ensure_builtin(auth):
    """Create the fixed entry disabled; preserve existing routing and enabled state."""
    with BUILTIN_LOCK:
        code, value = api('/api/upstream-proxies',auth)
        if code != 200: raise Invalid('无法读取前置代理')
        existing = next((p for p in proxy_rows(value) if p.get('id') == BUILTIN_ID),None)
        data = builtin_data(existing.get('enabled',False) if existing else False)
        if existing and all(existing.get(k,'') == data[k] for k in ('name','addr','username')):
            return
        code, _ = api('/api/upstream-proxies'+('/'+BUILTIN_ID if existing else ''),auth,data,
                      timeout=25,method='PUT' if existing else 'POST')
        if code not in (200,201): raise Invalid('内置代理初始化失败，请稍后重试')


def set_builtin_enabled(auth, enabled):
    """Connect/disconnect only the fixed local entry; never change country rules."""
    with BUILTIN_LOCK:
        code, value = api('/api/upstream-proxies', auth)
        if code != 200: raise Invalid('无法读取内置代理状态')
        existing = next((p for p in proxy_rows(value) if p.get('id') == BUILTIN_ID), None)
        data = builtin_data(enabled)
        if existing and all(existing.get(k, '') == v for k, v in data.items() if k != 'password'):
            return
        code, _ = api('/api/upstream-proxies'+('/'+BUILTIN_ID if existing else ''), auth, data,
                      timeout=25, method='PUT' if existing else 'POST')
        if code not in (200, 201): raise Invalid('内置代理关联失败，请重试')


def change_managed_connection(auth, action, payload):
    with PROXIES.lock:
        before = json.loads(json.dumps(PROXIES.state))
        PROXIES.change(action, payload)
        try:
            set_builtin_enabled(auth, PROXIES.state['enabled'])
        except Exception:
            PROXIES.commit(before)
            raise Invalid('内置代理状态同步失败，已恢复出口配置，请重试') from None


def initialize_builtin():
    web = yaml.safe_load(CONFIG.read_text())['web']
    code, value = api('/api/auth/login',body={'username':web['username'],'password':web['password']})
    if code != 200 or not value.get('token'): raise Invalid('本机登录尚未就绪')
    ensure_builtin('Bearer '+value['token'])


def api(path, token=None, body=None, timeout=10, method=None):
    conn = http.client.HTTPConnection(UPSTREAM_HOST, UPSTREAM_PORT, timeout=timeout)
    headers = {'Content-Type':'application/json'}
    if token:
        headers['Authorization'] = token
    try:
        conn.request(method or ('GET' if body is None else 'POST'), path, body=None if body is None else json.dumps(body).encode(), headers=headers)
        response = conn.getresponse()
        raw = response.read(2 * 1024 * 1024)
        try:
            value = json.loads(raw)
        except (ValueError, UnicodeError):
            value = {}
        return response.status, value
    finally:
        conn.close()


def devices_from(value):
    return value.get('devices', []) if isinstance(value, dict) else []


def send_sms(task):
    # Read the existing configuration each time, including password rotations.
    # No account secret or user token is persisted in the scheduler database.
    try:
        config = yaml.safe_load(CONFIG.read_text())
        task['device_name'] = next((d.get('name') or task['device_id'] for d in config.get('devices', [])
                                    if d.get('id') == task['device_id']), task['device_id'])
        web = config['web']
        status, login = api('/api/auth/login', body={'username':web['username'], 'password':web['password']})
    except Exception:
        return 'failed', '无法登录本机 VoHive，已暂停，请检查应用配置'
    if status != 200 or not login.get('token'):
        return 'failed', '本机 VoHive 登录失败，已暂停'
    token = 'Bearer ' + login['token']
    status, value = api('/api/devices', token)
    device = next((d for d in devices_from(value) if d.get('id') == task['device_id']), None)
    if device:
        task['device_name'] = device.get('name') or task['device_id']
    if status != 200 or not device or not device.get('running') or not device.get('sms_enabled'):
        return 'failed', '发信设备不存在、未在线或未启用短信，已暂停'
    try:
        status, value = api('/api/sms/send', token, {'device_id':task['device_id'], 'phone':task['phone'], 'message':task['message']}, timeout=120)
    except Exception:
        return 'unknown', '发送结果未知，已暂停，请核对短信记录后再开始'
    task['send_result'] = {k:value[k] for k in ('message_id','delivery_state','parts_total') if k in value}
    if not 200 <= status < 300 or value.get('status') in ('error', 'failed') or value.get('delivery_state') == 'failed':
        return 'failed', f'发送接口未成功（HTTP {status}），已暂停，请查看短信记录'
    return 'success', '已提交发送；实际投递状态请在短信中心查看'


def query_sms_delivery(message_id):
    if not isinstance(message_id, str) or not message_id or len(message_id) > 512:
        return None
    web = yaml.safe_load(CONFIG.read_text())['web']
    status, login = api('/api/auth/login', body={'username':web['username'],'password':web['password']})
    if status != 200 or not login.get('token'):
        return None
    status, result = api('/api/sms/delivery/'+quote(message_id,safe=''), 'Bearer '+login['token'])
    return result.get('delivery') if status == 200 and isinstance(result,dict) else None


class Handler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'

    def setup(self):
        super().setup()
        self.connection.settimeout(180)

    def log_message(self, *args):
        pass

    def json(self, code, value):
        body = json.dumps(value, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header('Content-Type','application/json; charset=utf-8')
        self.send_header('Cache-Control','no-store')
        self.send_header('Connection','close')
        self.close_connection = True
        self.send_header('Content-Length', str(len(body)))
        self.send_header('X-Content-Type-Options', 'nosniff')
        self.end_headers()
        if self.command != 'HEAD':
            self.wfile.write(body)

    def body(self, limit=32 * 1024 * 1024):
        # Reject ambiguous framing instead of allowing request smuggling.
        if self.headers.get('Transfer-Encoding'):
            raise Invalid('不支持分块上传')
        value = self.headers.get('Content-Length', '0')
        if not value.isdigit() or len(self.headers.get_all('Content-Length', [])) > 1:
            raise Invalid('请求长度无效')
        length = int(value)
        if length > limit:
            raise Invalid('请求过大')
        return self.rfile.read(length)

    def system_metadata(self):
        auth = self.headers.get('Authorization', '')
        status, _ = api('/api/devices', auth) if auth.startswith('Bearer ') and len(auth) <= 8192 else (401, {})
        if status != 200:
            return self.json(401 if status in (401, 403) else 503, {'error':'请重新登录后再试'})
        if self.command != 'GET':
            return self.json(405, {'error':'方法不支持'})
        try:
            build_time = json.loads((ASSETS / 'build-info.json').read_text()).get('build_time')
        except (OSError, ValueError, AttributeError):
            build_time = None
        return self.json(200, {'system_time':time.time(), 'build_time':build_time, 'config_path':str(CONFIG.absolute()),
                               'driver_version':driver_version(),
                               'proxy_version':proxy_version(PROXIES.binary) if PROXIES else '暂不可用'})

    def username_settings(self):
        auth = self.headers.get('Authorization', '')
        status, _ = api('/api/devices', auth) if auth.startswith('Bearer ') and len(auth) <= 8192 else (401, {})
        if status != 200:
            return self.json(401 if status in (401, 403) else 503, {'error':'请重新登录后再试'})
        if self.command == 'GET':
            return self.json(200, {'username':username_from(CONFIG)})
        if self.command != 'POST':
            return self.json(405, {'error':'方法不支持'})
        if ACCOUNT_RESTART is None:
            return self.json(503, {'error':'账号更新服务尚未就绪'})
        origin = self.headers.get('Origin')
        if origin and urlsplit(origin).netloc != self.headers.get('Host'):
            return self.json(403, {'error':'不允许跨站修改账号'})
        if self.headers.get('Content-Type','').split(';')[0] != 'application/json':
            return self.json(415, {'error':'需要 JSON 请求'})
        try:
            payload = json.loads(self.body(limit=4096))
            if not isinstance(payload, dict):
                raise ValueError('请求格式无效')
            def authenticate(username, password):
                code, result = api('/api/auth/login', body={'username':username, 'password':password})
                return code == 200 and bool(result.get('token'))
            username = rename_username(CONFIG, payload.get('username'), payload.get('current_password'), authenticate)
        except PermissionError as exc:
            return self.json(403, {'error':str(exc)})
        except (ValueError, StopIteration):
            return self.json(400, {'error':'请检查新用户名和当前密码；用户名需为 3–64 位字母、数字或 _ . @ -，且不能与当前用户名相同'})
        # Only an explicit successful username save restarts the existing service.
        # Docker's restart policy applies the changed login name to the upstream core.
        try:
            self.json(200, {'username':username, 'restart_required':True})
        finally:
            # The configuration is committed even if the caller disconnects.
            threading.Timer(2, ACCOUNT_RESTART).start()

    def managed_proxy(self, path):
        auth = self.headers.get('Authorization', '')
        if not auth.startswith('Bearer ') or len(auth) > 8192:
            return self.json(401, {'error':'请先登录 VoHive'})
        status, device_data = api('/api/devices', auth)
        if status != 200:
            return self.json(401 if status in (401,403) else 503, {'error':'无法验证登录状态，请重新登录或稍后重试'})
        if PROXIES is None:
            return self.json(503, {'error':'代理管理正在启动'})
        if self.command == 'GET' and path == '/api/managed-proxy/public-ip':
            value=dict(EGRESS.snapshot(PROXIES,force=urlsplit(self.path).query=='refresh=1'))
            rules_code,rules=api('/api/upstream-proxy-country-rules',auth)
            proxy_code,proxies=api('/api/upstream-proxies',auth)
            value['devices']=device_egress(devices_from(device_data),rules if rules_code==200 else None,proxy_rows(proxies) if proxy_code==200 else None,value)
            return self.json(200,value)
        if self.command == 'GET' and path == '/api/managed-proxy':
            return self.json(200, PROXIES.snapshot())
        if self.command != 'POST':
            return self.json(405, {'error':'方法不支持'})
        if self.headers.get('Content-Type','').split(';')[0] != 'application/json':
            return self.json(415, {'error':'仅接受文字 JSON，不接受图片上传'})
        origin = self.headers.get('Origin')
        if origin and urlsplit(origin).netloc != self.headers.get('Host'):
            return self.json(403, {'error':'不允许跨站修改代理'})
        try:
            payload = json.loads(self.body(4 * 1024 * 1024))
            if not isinstance(payload,dict): raise Invalid('请求格式无效')
            action = path.removeprefix('/api/managed-proxy/')
            if action == 'import': PROXIES.import_source(payload)
            elif action in ('select','pause'): change_managed_connection(auth,action,payload)
            elif action in ('refresh','delete'): PROXIES.change(action,payload)
            elif action == 'test': return self.json(200, PROXIES.test(payload.get('node_id')))
            elif action == 'attach':
                with PROXIES.lock:
                    PROXIES.check_version(payload)
                    if not PROXIES.state['enabled']: raise Invalid('请先选择并启用一个节点')
                    # Only this explicitly managed entry is updated; country rules stay user-controlled.
                    proxy_id = 'vohive-mihomo'
                    code, existing = api('/api/upstream-proxies',auth)
                    if code != 200: raise Invalid('无法读取现有前置代理')
                    rows = existing if isinstance(existing,list) else existing.get('proxies',[]) if isinstance(existing,dict) else []
                    present = any(p.get('id') == proxy_id for p in rows)
                    data = builtin_data(True)
                    code, value = api('/api/upstream-proxies'+('/'+proxy_id if present else ''),auth,data,timeout=25,method='PUT' if present else 'POST')
                    if code not in (200,201): raise Invalid('关联失败，请检查内置代理是否正常运行')
                return self.json(200, {'message':'已关联前置代理，请在下方为它绑定运营商国家'})
            else: return self.json(404, {'error':'接口不存在'})
            return self.json(200, PROXIES.snapshot())
        except Conflict as exc:
            return self.json(409, {'error':str(exc)})
        except Invalid as exc:
            return self.json(400, {'error':str(exc)})
        except (ValueError,TypeError,KeyError,RecursionError):
            return self.json(400, {'error':'节点或请求格式无效'})

    def upstream_proxy(self, path):
        """Protect the built-in record at the API boundary as well as in the UI."""
        auth = self.headers.get('Authorization','')
        if not auth.startswith('Bearer ') or len(auth)>8192:
            return self.json(401,{'error':'请先登录 VoHive'})
        status, _ = api('/api/devices',auth)
        if status != 200:
            return self.json(401 if status in (401,403) else 503,{'error':'无法验证登录状态'})
        fixed = path.rstrip('/') == '/api/upstream-proxies/'+BUILTIN_ID
        if fixed and self.command == 'DELETE':
            return self.json(403,{'error':'内置代理为固定项，不可删除；可通过列表开关停用'})
        if self.command == 'GET' and path.rstrip('/') == '/api/upstream-proxies':
            code, value = api('/api/upstream-proxies',auth)
            if code != 200: return self.json(code,value)
            rows = proxy_rows(value)
            existing = next((p for p in rows if p.get('id')==BUILTIN_ID),None)
            rows = [existing or builtin_data()]+[p for p in rows if p.get('id')!=BUILTIN_ID]
            return self.json(200,rows if isinstance(value,list) else dict(value,proxies=rows))
        if self.command in ('POST','PUT','PATCH'):
            if self.headers.get('Content-Type','').split(';')[0] != 'application/json':
                return self.json(415,{'error':'仅接受 JSON'})
            origin=self.headers.get('Origin')
            if origin and urlsplit(origin).netloc != self.headers.get('Host'):
                return self.json(403,{'error':'不允许跨站修改代理'})
            try: payload=json.loads(self.body(1024*1024))
            except (ValueError,UnicodeError): return self.json(400,{'error':'请求格式无效'})
            if not isinstance(payload,dict): return self.json(400,{'error':'请求格式无效'})
            if fixed or payload.get('id') == BUILTIN_ID:
                if not fixed or self.command != 'PUT':
                    return self.json(403,{'error':'内置代理为固定项，请使用列表启用开关'})
                if type(payload.get('enabled')) is not bool:
                    return self.json(400,{'error':'启用状态必须为布尔值'})
                expected=builtin_data(payload['enabled'])
                if any(k in payload and payload[k]!=expected[k] for k in ('id','name','addr','username','password')):
                    return self.json(400,{'error':'内置代理仅可修改启用状态'})
                with BUILTIN_LOCK:
                    ensure_builtin(auth)
                    code,value=api('/api/upstream-proxies/'+BUILTIN_ID,auth,expected,timeout=25,method='PUT')
            else:
                code,value=api(self.path,auth,payload,timeout=25,method=self.command)
            return self.json(code,value)
        if fixed and self.command not in ('GET','HEAD'):
            return self.json(405,{'error':'内置代理仅可修改启用状态'})
        return self.proxy()

    def scheduler(self, path):
        auth = self.headers.get('Authorization', '')
        if not auth.startswith('Bearer ') or len(auth) > 8192:
            return self.json(401, {'error':'请先登录 VoHive'})
        try:
            status, device_data = api('/api/devices', auth)
        except Exception:
            return self.json(503, {'error':'VoHive 正在启动，请稍后重试'})
        if status in (401,403):
            return self.json(status, {'error':'登录已失效，请重新登录'})
        if status != 200:
            return self.json(503, {'error':'暂时无法验证登录状态'})
        try:
            if self.command == 'GET':
                if path == '/api/schedules':
                    return self.json(200, {'tasks':STORE.list(), 'server_time':int(time.time()), 'timezone':'Asia/Shanghai'})
                if path == '/api/schedules/devices':
                    allowed = ('id','name','running','sms_enabled')
                    return self.json(200, {'devices':[{k:d.get(k) for k in allowed} for d in devices_from(device_data)]})
                parts = path.split('/')
                if len(parts) == 5 and parts[4] == 'history':
                    return self.json(200, {'runs':STORE.history(parts[3])})
                return self.json(404, {'error':'接口不存在'})
            if self.command not in ('POST','PUT','DELETE'):
                return self.json(405, {'error':'方法不支持'})
            if self.headers.get('Content-Type','').split(';')[0] != 'application/json':
                raise Invalid('仅接受 JSON 请求')
            origin = self.headers.get('Origin')
            if origin and urlsplit(origin).netloc != self.headers.get('Host'):
                return self.json(403, {'error':'不允许跨站修改任务'})
            raw = self.body()
            if len(raw) > 16384:
                raise Invalid('任务内容过大')
            payload = json.loads(raw)
            if not isinstance(payload, dict):
                raise Invalid('任务格式无效')
            parts = path.split('/')
            if self.command == 'POST' and path == '/api/schedules':
                if payload.get('device_id') not in [d.get('id') for d in devices_from(device_data)]:
                    raise Invalid('发信设备不存在，请重新选择')
                task_id = STORE.create(payload)
                return self.json(201, {'id':task_id})
            if len(parts) == 4 and self.command in ('PUT','DELETE'):
                action = 'edit' if self.command == 'PUT' else 'delete'
                if action == 'edit' and payload.get('device_id') not in [d.get('id') for d in devices_from(device_data)]:
                    raise Invalid('发信设备不存在，请重新选择')
                STORE.change(parts[3], action, payload)
            elif len(parts) == 5 and self.command == 'POST' and parts[4] in ('start','pause'):
                STORE.change(parts[3],parts[4],payload)
            else:
                return self.json(404, {'error':'接口不存在'})
            return self.json(200, {'status':'ok'})
        except Conflict as exc:
            return self.json(409, {'error':str(exc)})
        except (Invalid, ValueError) as exc:
            return self.json(400, {'error':str(exc)[:200]})
        except KeyError:
            return self.json(404, {'error':'任务不存在'})

    @staticmethod
    def _sms_time(value):
        try:
            return datetime.fromisoformat(str(value or '').replace('Z', '+00:00')).timestamp()
        except (ValueError, TypeError, OverflowError):
            return 0

    def sms_history(self, path):
        """Merge imported archives with the upstream SMS history.

        This path never invokes the send endpoint. Imported messages are stored
        with negative browser-facing IDs, allowing deletion without colliding
        with IDs owned by the upstream VoHive database.
        """
        auth = self.headers.get('Authorization', '')
        if not auth.startswith('Bearer ') or len(auth) > 8192:
            return self.json(401, {'error':'请先登录 VoHiveX'})
        status, device_data = api('/api/devices', auth)
        if status != 200:
            return self.json(401 if status in (401, 403) else 503,
                             {'error':'无法验证登录状态，请重新登录或稍后重试'})
        if SMS_ARCHIVE is None:
            return self.json(503, {'error':'短信归档服务正在启动'})
        query = parse_qs(urlsplit(self.path).query, keep_blank_values=True)

        if path == '/api/sms/archive/import':
            if self.command != 'POST':
                return self.json(405, {'error':'方法不支持'})
            if self.headers.get('Content-Type','').split(';')[0] != 'application/json':
                return self.json(415, {'error':'仅接受 JSON 短信归档'})
            origin = self.headers.get('Origin')
            if origin and urlsplit(origin).netloc != self.headers.get('Host'):
                return self.json(403, {'error':'不允许跨站导入短信'})
            try:
                payload = json.loads(self.body(16 * 1024 * 1024))
                if not isinstance(payload, dict) or not isinstance(payload.get('messages'), list):
                    raise ValueError('归档格式无效')
                rows = payload['messages']
                if not rows or len(rows) > 20000 or any(not isinstance(row, dict) for row in rows):
                    raise ValueError('归档必须包含 1–20000 条短信')
                device_id = str(payload.get('device_id') or '').strip()
                device = next((row for row in devices_from(device_data) if row.get('id') == device_id), None)
                if device is None:
                    raise ValueError('导入设备不存在，请重新选择')
                inserted = SMS_ARCHIVE.import_messages(device, rows)
                return self.json(200, {'inserted':inserted, 'skipped':len(rows)-inserted})
            except (ValueError, TypeError, OverflowError) as exc:
                return self.json(400, {'error':str(exc)[:200]})

        if self.command == 'GET' and path == '/api/sms/contacts':
            code, value = api(self.path, auth, timeout=25)
            if code != 200:
                return self.json(code, value)
            upstream = value if isinstance(value, list) else []
            device_id = (query.get('device_id') or [None])[0]
            merged = {}
            for row in upstream + SMS_ARCHIVE.contacts(device_id):
                if not isinstance(row, dict):
                    continue
                key = (str(row.get('imsi') or ''), str(row.get('peer') or ''))
                old = merged.get(key)
                if old is None or self._sms_time(row.get('last_timestamp')) >= self._sms_time(old.get('last_timestamp')):
                    merged[key] = row
            limit = max(1, min(int((query.get('limit') or ['200'])[0]), 500))
            rows = sorted(merged.values(), key=lambda row:self._sms_time(row.get('last_timestamp')), reverse=True)
            return self.json(200, rows[:limit])

        if self.command == 'GET' and path == '/api/sms/thread':
            code, value = api(self.path, auth, timeout=25)
            if code != 200:
                return self.json(code, value)
            peer = str((query.get('peer') or [''])[0]).strip()
            if not peer:
                return self.json(400, {'error':'缺少短信联系人'})
            device_id = (query.get('device_id') or [None])[0]
            imsi = (query.get('imsi') or [None])[0]
            limit = max(1, min(int((query.get('limit') or ['200'])[0]), 500))
            before_ts = (query.get('before_ts') or [None])[0]
            before_id = (query.get('before_id') or [None])[0]
            archive = SMS_ARCHIVE.messages(peer, device_id, imsi, limit, before_ts, before_id)
            rows = [row for row in (value if isinstance(value, list) else []) if isinstance(row, dict)] + archive
            rows.sort(key=lambda row:(self._sms_time(row.get('timestamp')), int(row.get('id') or 0)))
            return self.json(200, rows[-limit:])

        if self.command == 'DELETE' and path.startswith('/api/sms/messages/'):
            try:
                message_id = int(path.rsplit('/', 1)[-1])
            except ValueError:
                return self.proxy()
            if message_id >= 0:
                return self.proxy()
            result = SMS_ARCHIVE.delete_message(message_id)
            return self.json(200 if result['deleted'] else 404,
                             result if result['deleted'] else {'error':'短信不存在'})

        if self.command == 'DELETE' and path == '/api/sms/thread':
            peer = str((query.get('peer') or [''])[0]).strip()
            if not peer:
                return self.json(400, {'error':'缺少短信联系人'})
            device_id = (query.get('device_id') or [None])[0]
            imsi = (query.get('imsi') or [None])[0]
            archived = SMS_ARCHIVE.delete_thread(peer, device_id, imsi)
            code, value = api(self.path, auth, timeout=25, method='DELETE')
            if code not in (200, 204, 404) and not archived:
                return self.json(code, value)
            result = value if isinstance(value, dict) else {}
            result.update({'deleted_imported':archived, 'thread_empty':True})
            return self.json(200, result)

        return self.proxy()

    def proxy(self):
        body = self.body()
        conn = http.client.HTTPConnection(UPSTREAM_HOST, UPSTREAM_PORT, timeout=180)
        headers = {k:v for k,v in self.headers.items() if k.lower() not in HOP and k.lower() != 'content-length'}
        upgrade = self.headers.get('Upgrade','').lower() == 'websocket'
        if upgrade:
            headers.update(Connection='Upgrade', Upgrade='websocket')
        if body or self.headers.get('Content-Length'):
            headers['Content-Length'] = str(len(body))
        conn.request(self.command, self.path, body=body or None, headers=headers)
        response = conn.getresponse()
        self.send_response(response.status)
        for key, value in response.getheaders():
            if key.lower() not in HOP and key.lower() != 'content-length':
                self.send_header(key, value)
        if upgrade and response.status == 101:
            self.send_header('Connection','Upgrade')
            self.send_header('Upgrade','websocket')
            self.end_headers()
            self.wfile.flush()
            sockets = [self.connection, conn.sock]
            try:
                while True:
                    ready, _, _ = select.select(sockets, [], [], 60)
                    if not ready:
                        break
                    for src in ready:
                        data = src.recv(65536)
                        if not data:
                            return
                        (conn.sock if src is self.connection else self.connection).sendall(data)
            finally:
                conn.close()
                self.close_connection = True
            return
        # Response close framing handles both chunked SSE and regular responses.
        self.send_header('Connection','close')
        self.end_headers()
        self.close_connection = True
        try:
            if self.command != 'HEAD':
                while True:
                    data = response.read1(65536)
                    if not data:
                        break
                    self.wfile.write(data)
                    self.wfile.flush()
        finally:
            conn.close()

    def handle_request(self):
        path = urlsplit(self.path).path
        try:
            decoded = unquote(path)
            if decoded == '/api/settings/system':
                return self.system_metadata()
            if decoded == '/api/settings/username':
                return self.username_settings()
            if decoded.rstrip('/') == '/api/upstream-proxies' or decoded.startswith('/api/upstream-proxies/'):
                return self.upstream_proxy(decoded)
            if path == '/api/managed-proxy' or path.startswith('/api/managed-proxy/'):
                return self.managed_proxy(path)
            if path == '/api/schedules' or path.startswith('/api/schedules/'):
                return self.scheduler(path)
            if path == '/api/sms/archive/import' or path == '/api/sms/contacts' or path == '/api/sms/thread' or path.startswith('/api/sms/messages/'):
                return self.sms_history(path)
            static_name = 'index.html' if path in ('/','/index.html') else 'api/docs/index.html' if path.rstrip('/') == '/api/docs' else path.lstrip('/')
            static = ASSETS / static_name
            if self.command in ('GET','HEAD') and static.is_relative_to(ASSETS) and '..' not in path.split('/') and static.is_file():
                body = static.read_bytes()
                content_type = 'text/javascript' if static.suffix == '.js' else 'text/css' if static.suffix == '.css' else 'application/json' if static.suffix == '.json' else 'text/plain' if static.suffix == '.txt' or static.name in ('LICENSE','NOTICE') else 'text/html'
                image_type = {'.ico':'image/x-icon', '.png':'image/png', '.svg':'image/svg+xml', '.ttf':'font/ttf', '.woff2':'font/woff2'}.get(static.suffix)
                self.send_response(200)
                self.send_header('Content-Type',image_type or content_type+'; charset=utf-8')
                self.send_header('Content-Length',str(len(body)))
                self.send_header('Cache-Control','no-cache' if static.suffix == '.html' else 'public, max-age=31536000, immutable')
                self.send_header('X-Content-Type-Options','nosniff')
                self.end_headers()
                if self.command != 'HEAD':
                    self.wfile.write(body)
                return
            self.proxy()
        except (BrokenPipeError, ConnectionResetError):
            self.close_connection = True
        except Invalid as exc:
            self.close_connection = True
            self.json(400, {'error':str(exc)})
        except Exception:
            self.close_connection = True
            self.json(502, {'error':'服务暂时不可用，请稍后重试'})

    do_GET = do_HEAD = do_POST = do_PUT = do_PATCH = do_DELETE = do_OPTIONS = handle_request


if __name__ == '__main__':
    os.umask(0o077)
    ACCOUNT_RESTART = lambda: os.kill(os.getppid(), signal.SIGTERM)
    STORE = Store(os.environ.get('SCHEDULER_DB','/app/data/scheduled-sms.sqlite3'))
    SMS_ARCHIVE = SmsArchive(os.environ.get('SMS_ARCHIVE_DB','/app/data/imported-sms.sqlite3'))
    PROXIES = Manager(os.environ.get('PROXY_DATA','/app/data/managed-proxy'), os.environ.get('MIHOMO_BINARY','/opt/vohivex/proxy/mihomo'))
    try: PROXIES.start()
    except Exception: PROXIES.error = '代理核心未启动，请检查容器'
    worker = Worker(STORE, send_sms)
    notifier = NotificationWorker(STORE, Dispatcher(lambda: yaml.safe_load(CONFIG.read_text()) or {}), worker.stop)
    notification_thread = threading.Thread(target=notifier.run, daemon=True)
    notification_thread.start()
    delivery_worker = DeliveryWorker(STORE, query_sms_delivery, worker.stop)
    delivery_thread = threading.Thread(target=delivery_worker.run, daemon=True)
    delivery_thread.start()
    thread = threading.Thread(target=worker.run, daemon=True)
    thread.start()
    def maintain_proxy():
        builtin_ready = False
        while not worker.stop.is_set():
            if not builtin_ready:
                try: initialize_builtin(); builtin_ready = True
                except Exception: pass  # Retry while the local core is starting; never log credentials.
            PROXIES.maintain()
            if worker.stop.wait(5): break
    proxy_thread = threading.Thread(target=maintain_proxy,daemon=True)
    proxy_thread.start()
    server = ThreadingHTTPServer(('0.0.0.0', int(os.environ.get('SCHEDULER_PORT','7575'))), Handler)
    server.daemon_threads = True
    def shutdown(*_):
        worker.stop.set()
        threading.Thread(target=server.shutdown, daemon=True).start()
    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)
    print('Scheduled SMS service listening on port', server.server_port, flush=True)
    server.serve_forever(poll_interval=0.25)
    server.server_close()
    PROXIES.close()
    thread.join(timeout=5)
    notification_thread.join(timeout=5)
    delivery_thread.join(timeout=5)
