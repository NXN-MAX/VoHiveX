"""Scheduled SMS result delivery through the user's enabled notification channels.

No automatic retries: an ambiguous response must never resend an SMS or duplicate
a push. Transport errors are recorded without secrets, URLs or message content.
"""
from datetime import datetime
from email.message import EmailMessage
import hashlib
import hmac
import json
import re
import smtplib
import ssl
import threading
from urllib.parse import quote, urlsplit
from urllib.request import Request, ProxyHandler, HTTPRedirectHandler, build_opener


CHANNELS = ('telegram', 'feishu', 'qq', 'bark', 'email', 'pushplus', 'webhook')
STATUS = {'success': '已提交发送（实际投递状态请查看短信中心）',
          'failed': '发送失败', 'unknown': '发送结果未知，请核对短信记录'}


def result_title(event):
    return '定时短信发送结果' if event.get('phase') == 'delivery' else '定时短信执行结果'


def format_result(event):
    timestamp = datetime.fromtimestamp(event['finished_at']).astimezone().isoformat(sep=' ', timespec='seconds')
    return '\n'.join([
        result_title(event),
        '任务：' + event['name'],
        '发信设备：' + (event.get('device_name') or event['device_id']),
        '收信号码：' + event['phone'],
        '时间：' + timestamp,
        '状态：' + ('发送成功' if event.get('phase') == 'delivery' and event['status'] == 'success'
                   else STATUS.get(event['status'], event['status'])),
        '详情：' + event['detail'],
        '短信内容：', event['message'],
    ])


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def post(url, payload, headers=None, timeout=10, proxy=None, secret=None):
    if urlsplit(url).scheme not in ('http', 'https'):
        raise ValueError('invalid endpoint')
    raw = json.dumps(payload, ensure_ascii=False, separators=(',', ':')).encode()
    headers = dict(headers or {})
    headers['Content-Type'] = 'application/json'
    if secret:
        headers['X-Vohive-Signature'] = 'sha256=' + hmac.new(secret.encode(), raw, hashlib.sha256).hexdigest()
    if proxy and urlsplit(proxy).scheme not in ('http', 'https'):
        raise ValueError('HTTP proxy required')
    # Do not pick up unrelated host proxy environment variables.
    opener = build_opener(ProxyHandler({'http':proxy,'https':proxy} if proxy else {}), NoRedirect())
    with opener.open(Request(url, data=raw, headers=headers, method='POST'), timeout=timeout) as response:
        if not 200 <= response.status < 300:
            raise ValueError('HTTP failure')
        data = response.read(1024 * 1024 + 1)
        if len(data) > 1024 * 1024:
            raise ValueError('response too large')
        try:
            return json.loads(data)
        except (ValueError, UnicodeError):
            return {}


def recipients(value):
    values = value if isinstance(value, list) else re.split(r'[,，;；\s]+', str(value or ''))
    return list(dict.fromkeys(str(v).strip() for v in values if str(v).strip()))


def telegram_parts(text):
    # Telegram measures length in UTF-16 units; preserve even a 2000-emoji SMS.
    current, length = [], 0
    for char in text:
        size = 2 if ord(char) > 0xffff else 1
        if length + size > 4000:
            yield ''.join(current)
            current, length = [], 0
        current.append(char)
        length += size
    if current:
        yield ''.join(current)


def accepted(result, key, value):
    if not isinstance(result, dict) or result.get(key) != value:
        raise ValueError('channel rejected message')


class Dispatcher:
    def __init__(self, config_loader, transport=post):
        self.config_loader, self.post = config_loader, transport

    def __call__(self, event):
        config = self.config_loader()
        text = format_result(event)
        results = {}
        for channel in CHANNELS:
            settings = config.get(channel) or {}
            if not settings.get('enabled'):
                continue
            try:
                failures = getattr(self, channel)(settings, text, event)
                results[channel] = 'failed' if failures else 'success'
            except Exception:
                results[channel] = 'failed'
        return results

    @staticmethod
    def each(targets, send):
        if not targets:
            raise ValueError('no recipients')
        failed = 0
        for target in targets:
            try:
                send(target)
            except Exception:
                failed += 1
        return failed

    def telegram(self, cfg, text, event):
        template = cfg.get('base_url') or 'https://api.telegram.org/bot%s/%s'
        token = cfg['bot_token']
        url = template % (token, 'sendMessage') if '%s' in template else template.rstrip('/') + '/bot' + token + '/sendMessage'
        # chat_id is the configured push destination; admin_id grants commands only.
        if not cfg.get('chat_id'):
            raise ValueError('no chat')
        for part in telegram_parts(text):
            result = self.post(url, {'chat_id':cfg['chat_id'],'text':part}, proxy=cfg.get('proxy'))
            accepted(result, 'ok', True)

    def feishu(self, cfg, text, event):
        token = self.post('https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal',
                          {'app_id':cfg['app_id'],'app_secret':cfg['app_secret']})
        accepted(token, 'code', 0)
        headers = {'Authorization':'Bearer ' + token['tenant_access_token']}
        def send(target):
            result = self.post('https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id',
                               {'receive_id':target,'msg_type':'text','content':json.dumps({'text':text},ensure_ascii=False),
                                'uuid':hashlib.sha256((event['run_id']+event.get('phase','execution')+target).encode()).hexdigest()[:32]}, headers=headers)
            accepted(result, 'code', 0)
        return self.each(recipients(cfg.get('chat_ids') or cfg.get('chat_id')), send)

    def qq(self, cfg, text, event):
        token = self.post('https://bots.qq.com/app/getAppAccessToken',
                          {'appId':cfg['app_id'],'clientSecret':cfg['app_secret']})
        headers = {'Authorization':'QQBot ' + token['access_token'],'X-Union-Appid':str(cfg['app_id'])}
        targets = [('groups', x) for x in recipients(cfg.get('group_ids'))]
        targets += [('users', x) for x in recipients(cfg.get('direct_ids'))]
        def send(target):
            kind, ident = target
            result = self.post('https://api.sgroup.qq.com/v2/'+kind+'/'+quote(ident,safe='')+'/messages',
                               {'content':text,'msg_type':0}, headers=headers)
            if not isinstance(result, dict) or not result.get('id'):
                raise ValueError('QQ rejected message')
        return self.each(targets, send)

    def bark(self, cfg, text, event):
        payload = {'title':result_title(event),'body':text,
                   **{k:cfg[k] for k in ('group','icon','level') if cfg.get(k)}}
        def send(url):
            accepted(self.post(url, payload), 'code', 200)
        return self.each(recipients(cfg.get('urls')), send)

    def email(self, cfg, text, event):
        targets = recipients(cfg.get('to_addresses'))
        if not targets:
            raise ValueError('no email recipients')
        message = EmailMessage()
        message['Subject'] = 'VoHiveX · ' + result_title(event)
        message['From'] = cfg.get('from_address') or cfg.get('username')
        message['To'] = ', '.join(targets)
        message.set_content(text)
        context = ssl.create_default_context()
        secure = bool(cfg.get('use_ssl'))
        port = int(cfg.get('smtp_port') or (465 if secure else 587))
        cls = smtplib.SMTP_SSL if secure else smtplib.SMTP
        kwargs = {'context':context} if secure else {}
        with cls(cfg['smtp_host'], port, timeout=10, **kwargs) as smtp:
            if not secure:
                smtp.ehlo()
                if smtp.has_extn('starttls'):
                    smtp.starttls(context=context)
                    smtp.ehlo()
            if cfg.get('username'):
                smtp.login(cfg['username'], cfg.get('password') or '')
            refused = smtp.send_message(message)
            if refused:
                raise ValueError('some recipients refused')

    def pushplus(self, cfg, text, event):
        result = self.post('https://www.pushplus.plus/send',
                           {'token':cfg['token'],'title':result_title(event),'content':text,'template':'txt',
                            'topic':cfg.get('topic') or '', 'channel':cfg.get('channel') or 'wechat'})
        accepted(result, 'code', 200)

    def webhook(self, cfg, text, event):
        # Include the complete result independent of the incoming-SMS text template.
        payload = {'event':'scheduled_sms.delivery' if event.get('phase') == 'delivery' else 'scheduled_sms.completed',
                   'text':text,'title':result_title(event),
                   'device_label':event.get('device_name') or event['device_id'], **event}
        headers = {k:str(v) for k,v in (cfg.get('headers') or {}).items()
                   if k.lower() not in ('host','content-length','connection','transfer-encoding','x-vohive-signature')}
        timeout = max(1, min(30, float(cfg.get('timeout_ms') or 5000) / 1000))
        return self.each(recipients(cfg.get('urls')),
                         lambda url: self.post(url,payload,headers=headers,secret=cfg.get('secret'),timeout=timeout))


class NotificationWorker:
    def __init__(self, store, dispatcher, stop=None):
        self.store, self.dispatcher = store, dispatcher
        self.stop = stop if stop is not None else threading.Event()

    def tick(self):
        event = self.store.claim_notification()
        if not event:
            return
        notification_id = event.pop('_notification_id', event['run_id'])
        try:
            result = self.dispatcher(event)
            state = 'failed' if 'failed' in result.values() else 'sent' if result else 'disabled'
            detail = json.dumps(result, ensure_ascii=False) if result else '未启用消息推送渠道'
        except Exception:
            state, detail = 'failed', '无法读取或处理消息推送配置'
        self.store.finish_notification(notification_id, state, detail)
        # Never log endpoint URLs, credentials, phone numbers or SMS contents.
        print('Scheduled SMS notification:', notification_id, state, detail, flush=True)

    def run(self):
        self.store.recover_notifications()
        while not self.stop.is_set():
            try:
                self.tick()
            except Exception as exc:
                print('Notification worker failed:', type(exc).__name__, flush=True)
            self.stop.wait(0.25)
