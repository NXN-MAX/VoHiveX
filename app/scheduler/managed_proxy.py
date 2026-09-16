"""Private subscription store and a loopback-only Mihomo child; no host networking changes."""
import base64
import copy
import hashlib
import http.client
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import socket
import ssl
import subprocess
import threading
import time
from urllib.parse import urlsplit, urljoin, unquote, parse_qs, quote
import uuid
import yaml
from engine import Invalid, Conflict

MAX_BYTES = 4 * 1024 * 1024
SOCKS_PORT = 17890
CONTROLLER_PORT = 17891
LOOPBACK = '127.0.0.1'
TYPES = {'vmess', 'vless', 'anytls', 'trojan', 'ss', 'hysteria2', 'tuic', 'socks5'}
COMMON = {'type','name','server','port','udp','uuid','password','username','cipher','alterId','tls','servername','sni','skip-cert-verify','client-fingerprint','fingerprint','alpn','network','flow','packet-encoding','encryption','obfs','obfs-password','up','down','congestion-controller','udp-relay-mode','reduce-rtt','disable-sni','heartbeat-interval','idle-session-check-interval','idle-session-timeout','min-idle-session'}
NESTED = {'ws-opts':{'path','headers','max-early-data','early-data-header-name'}, 'grpc-opts':{'grpc-service-name'}, 'reality-opts':{'public-key','short-id'}, 'http-opts':{'method','path','headers'}, 'h2-opts':{'host','path'}}


def clean_name(value):
    if not isinstance(value,str): raise Invalid('名称格式无效')
    return re.sub(r'[\x00-\x1f\x7f]', '', value).strip()[:100] or '未命名'


def decode64(value):
    try: return base64.urlsafe_b64decode(value + '=' * (-len(value) % 4)).decode('utf-8')
    except Exception: raise Invalid('节点编码无效') from None


def uri_node(text):
    u=urlsplit(text); typ=u.scheme.lower(); q={k:v[0] for k,v in parse_qs(u.query).items()}
    if typ=='vmess':
        try: v=json.loads(decode64(text[8:]))
        except Exception: raise Invalid('VMess 链接格式无效') from None
        n={'type':'vmess','name':v.get('ps','VMess'),'server':v.get('add'),'port':v.get('port'),'uuid':v.get('id'),'alterId':int(v.get('aid') or 0),'cipher':v.get('scy') or 'auto','udp':True,'tls':v.get('tls')=='tls'}
        network=v.get('net','tcp')
        if network not in ('tcp','ws','grpc','h2','http'): raise Invalid('暂不支持此 VMess 传输方式')
        if network!='tcp': n['network']=network
        if v.get('sni'): n['servername']=v['sni']
        if network=='ws': n['ws-opts']={'path':v.get('path') or '/', 'headers':{'Host':v.get('host') or v.get('add')}}
        if network=='grpc': n['grpc-opts']={'grpc-service-name':v.get('path') or ''}
        return n
    if typ=='hy2': typ='hysteria2'
    if typ not in TYPES: raise Invalid('不支持的节点链接类型')
    if typ=='ss':
        body=text[5:].split('#',1)[0].split('?',1)[0].rstrip('/')
        if '@' not in body: body=decode64(body)
        cred, sep, address=body.rpartition('@')
        if not sep: raise Invalid('Shadowsocks 链接格式无效')
        if ':' not in cred: cred=decode64(cred)
        cipher,sep,password=unquote(cred).partition(':'); peer=urlsplit('ss://'+address)
        if q.get('plugin'): raise Invalid('暂不支持带插件的 Shadowsocks 链接')
        return {'type':'ss','name':unquote(u.fragment) or 'Shadowsocks','server':peer.hostname,'port':peer.port,'cipher':cipher,'password':password,'udp':True}
    n={'type':typ,'name':unquote(u.fragment) or typ.upper(),'server':u.hostname,'port':u.port or 443,'udp':True}
    if typ in ('vless','tuic'): n['uuid']=unquote(u.username or '')
    if typ in ('trojan','anytls','hysteria2'): n['password']=unquote(u.username or '')
    if typ in ('tuic','socks5'):
        n['password']=unquote(u.password or '')
        if typ=='socks5': n['username']=unquote(u.username or '')
    security=q.get('security','tls' if typ in ('trojan','anytls') else '')
    if security in ('tls','reality') or typ in ('anytls','trojan','hysteria2','tuic'): n['tls']=True
    if q.get('sni') or q.get('peer'): n['servername']=q.get('sni') or q['peer']
    if q.get('fp'): n['client-fingerprint']=q['fp']
    if q.get('alpn'): n['alpn']=q['alpn'].split(',')
    if q.get('flow'): n['flow']=q['flow']
    if q.get('allowInsecure')=='1' or q.get('insecure')=='1': n['skip-cert-verify']=True
    if security=='reality': n['reality-opts']={'public-key':q.get('pbk',''),'short-id':q.get('sid','')}
    network=q.get('type','tcp')
    if network not in ('tcp','ws','grpc','http','h2'): raise Invalid('暂不支持此传输方式')
    if network!='tcp': n['network']=network
    if network=='ws': n['ws-opts']={'path':q.get('path','/'),'headers':{'Host':q.get('host',u.hostname)}}
    if network=='grpc': n['grpc-opts']={'grpc-service-name':q.get('serviceName','')}
    if q.get('obfs'): n['obfs']=q['obfs']
    if q.get('obfs-password'): n['obfs-password']=q['obfs-password']
    return n


def sanitize(node):
    if not isinstance(node,dict) or node.get('type') not in TYPES: raise Invalid('包含暂不支持的节点类型')
    # Only plain data is accepted. File paths, dialer-proxy and provider/global settings never reach the core.
    try: encoded=json.dumps(node,ensure_ascii=False)
    except Exception: raise Invalid('节点参数无效') from None
    if len(encoded)>16000: raise Invalid('单个节点参数过大')
    n={k:v for k,v in node.items() if k in COMMON}
    for k,allowed in NESTED.items():
        if k in node:
            if not isinstance(node[k],dict): raise Invalid('传输参数格式无效')
            n[k]={a:b for a,b in node[k].items() if a in allowed}
    n['name']=clean_name(n.get('name','未命名'))
    server=n.get('server')
    if not isinstance(server,str) or not server or len(server)>253 or re.search(r'[\s/@?#\x00]',server): raise Invalid('节点服务器地址无效')
    try: port=int(n.get('port',0))
    except Exception: raise Invalid('节点端口无效') from None
    if not 1<=port<=65535: raise Invalid('节点端口无效')
    n['port']=port
    if 'udp' in n and not isinstance(n['udp'],bool): raise Invalid('UDP 参数无效')
    n.setdefault('udp',True)
    return n


def parse_nodes(text):
    if not isinstance(text,str) or len(text.encode())>MAX_BYTES: raise Invalid('订阅内容超过 4 MB')
    text=text.strip('\ufeff \r\n\t')
    if not text: raise Invalid('订阅没有节点')
    if re.match(r'^(vmess|vless|anytls|trojan|ss|hy2|hysteria2|tuic|socks5)://',text):
        raw=[uri_node(s.strip()) for s in text.splitlines() if s.strip()]
    else:
        try:
            for tok in yaml.scan(text):
                if isinstance(tok,(yaml.tokens.AliasToken,yaml.tokens.AnchorToken)): raise Invalid('不接受 YAML 引用，请使用普通节点列表')
            data=yaml.safe_load(text)
        except (yaml.YAMLError,RecursionError): raise Invalid('订阅格式无法解析') from None
        if isinstance(data,dict) and isinstance(data.get('proxies'),list): raw=data['proxies']
        elif isinstance(data,str) and re.fullmatch(r'[A-Za-z0-9_+/=\s-]+',data): return parse_nodes(decode64(''.join(data.split())))
        else: raise Invalid('请使用 Clash YAML、Base64 订阅或节点分享链接')
    if not 1<=len(raw)<=1000: raise Invalid('每个订阅支持 1–1000 个节点')
    nodes=[]; seen=set()
    for node in raw:
        n=sanitize(node)
        # Stable across refreshes; repeated display names remain separate when endpoints differ.
        ident=hashlib.sha256(json.dumps(n,sort_keys=True,ensure_ascii=False).encode()).hexdigest()[:24]
        if ident not in seen: nodes.append({'key':ident,'name':n['name'],'config':n});seen.add(ident)
    return nodes


def fetch_subscription(url):
    """HTTPS only, public destinations, DNS pinned per request; no proxy env, cache or remote converter."""
    for _ in range(4):
        try: u=urlsplit(url); port=u.port or 443
        except ValueError: raise Invalid('订阅地址无效') from None
        if u.scheme!='https' or not u.hostname or u.username or u.password or u.fragment or len(url)>4096: raise Invalid('请填写 HTTPS 订阅地址')
        try:
            addresses=socket.getaddrinfo(u.hostname,port,type=socket.SOCK_STREAM)
            if not addresses or any(not ipaddress.ip_address(a[4][0]).is_global for a in addresses): raise Invalid('订阅地址必须指向公网服务器')
            target=addresses[0][4][0]
            conn=http.client.HTTPSConnection(u.hostname,port,timeout=15)
            sock=socket.create_connection((target,port),timeout=15)
            conn.sock=ssl.create_default_context().wrap_socket(sock,server_hostname=u.hostname)
            path=u.path or '/'
            if u.query: path+='?'+u.query
            conn.request('GET',path,headers={'User-Agent':'clash.meta','Accept':'text/yaml,text/plain,*/*','Accept-Encoding':'identity','Cache-Control':'no-store'})
            r=conn.getresponse()
            if r.status in (301,302,303,307,308):
                location=r.getheader('Location','');conn.close();url=urljoin(url,location);continue
            if r.status!=200: raise Invalid('订阅下载失败（HTTP '+str(r.status)+'）')
            raw=r.read(MAX_BYTES+1)
            if len(raw)>MAX_BYTES: raise Invalid('订阅内容超过 4 MB')
            return raw.decode('utf-8-sig')
        except Invalid: raise
        except Exception: raise Invalid('无法获取订阅，请检查链接、证书或网络') from None
        finally:
            if 'conn' in locals(): conn.close()
    raise Invalid('订阅重定向次数过多')


def atomic_json(path,value):
    tmp=path.with_suffix('.tmp')
    fd=os.open(tmp,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
    with os.fdopen(fd,'w') as f:
        json.dump(value,f,ensure_ascii=False); f.flush(); os.fsync(f.fileno())
    os.replace(tmp,path)


class Manager:
    def __init__(self,directory,binary='/opt/vohivex/proxy/mihomo',port=SOCKS_PORT,controller=CONTROLLER_PORT):
        self.dir=Path(directory); self.dir.mkdir(parents=True,exist_ok=True,mode=0o700)
        self.path=self.dir/'state.json';self.config_path=self.dir/'config.json'
        self.binary=binary;self.port=port;self.controller=controller
        self.lock=threading.RLock();self.process=None;self.closed=False;self.last_restart=0
        self.state=json.loads(self.path.read_text()) if self.path.exists() else {'version':1,'sources':[],'selected':None,'enabled':False,'secret':secrets.token_hex(32)}
        if not self.path.exists(): atomic_json(self.path,self.state)
        self.error=''

    def nodes(self,state=None): return [n for s in (state or self.state)['sources'] for n in s['nodes']]

    @property
    def endpoint(self):
        return f'{LOOPBACK}:{self.port}'

    def snapshot(self):
        with self.lock:
            return {'version':self.state['version'],'enabled':self.state['enabled'],'selected':self.state['selected'],'running':bool(self.process and self.process.poll() is None),'error':self.error,'sources':[{'id':s['id'],'name':s['name'],'kind':s['kind'],'updated':s['updated'],'count':len(s['nodes'])} for s in self.state['sources']], 'nodes':[{'id':n['id'],'name':n['name'],'source_id':s['id'],'type':n['config']['type'],'udp':n['config'].get('udp',True)} for s in self.state['sources'] for n in s['nodes']], 'endpoint':self.endpoint}

    def config(self,state):
        selected=next((n for n in self.nodes(state) if n['id']==state['selected']),None)
        nodes=[dict(n['config'],name=n['id']) for n in self.nodes(state)]
        return {'socks-port':self.port,'bind-address':LOOPBACK,'allow-lan':False,'mode':'rule','log-level':'silent','ipv6':False,'external-controller':f'{LOOPBACK}:{self.controller}','secret':state['secret'],'profile':{'store-selected':False,'store-fake-ip':False},'geodata-mode':False,'geo-auto-update':False,'dns':{'enable':False},'tun':{'enable':False},'sniffer':{'enable':False},'proxies':nodes,'rules':['MATCH,'+(selected['id'] if selected and state['enabled'] else 'REJECT')]}

    def validate(self,config):
        path=self.dir/'check.json'; atomic_json(path,config)
        try:
            r=subprocess.run([self.binary,'-t','-d',str(self.dir),'-f',str(path)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=20)
            if r.returncode: raise Invalid('代理核心不接受部分节点参数，未应用本次修改')
        except FileNotFoundError: raise Invalid('代理核心尚未安装') from None
        finally: path.unlink(missing_ok=True)

    def stop_process(self):
        if self.process and self.process.poll() is None:
            self.process.terminate()
            try:self.process.wait(timeout=8)
            except subprocess.TimeoutExpired:self.process.kill();self.process.wait(timeout=3)
        self.process=None

    def start(self):
        with self.lock:
            if self.closed:return
            atomic_json(self.config_path,self.config(self.state));self.stop_process()
            self.process=subprocess.Popen([self.binary,'-d',str(self.dir),'-f',str(self.config_path)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            self.last_restart=time.monotonic()
            for _ in range(40):
                if self.process.poll() is not None:break
                try:
                    self.control('/version'); self.error=''; return
                except Exception:time.sleep(.1)
            self.stop_process();self.error='代理核心未启动，请重试或检查容器';raise Invalid(self.error)

    def close(self):
        with self.lock:self.closed=True;self.stop_process()

    def maintain(self):
        with self.lock:
            if not self.closed and (not self.process or self.process.poll() is not None) and time.monotonic()-self.last_restart>30:
                try:self.start()
                except Exception:self.error='代理核心未启动，请检查容器';self.last_restart=time.monotonic()

    def control(self,path):
        conn=http.client.HTTPConnection('127.0.0.1',self.controller,timeout=10)
        try:
            conn.request('GET',path,headers={'Authorization':'Bearer '+self.state['secret']})
            r=conn.getresponse();raw=r.read(1024*1024)
            if r.status!=200: raise Invalid('节点连接测试失败')
            return json.loads(raw)
        finally:conn.close()

    def check_version(self,payload):
        if type(payload.get('version')) is not int or payload['version']!=self.state['version']: raise Conflict('配置已变化，请刷新后重试')

    def commit(self,next_state):
        self.validate(self.config(next_state))
        old=self.state;next_state['version']=old['version']+1
        # Persist intent before process restart: on power loss startup uses this exact selection.
        atomic_json(self.path,next_state);self.state=next_state
        try:self.start()
        except Exception:
            atomic_json(self.path,old);self.state=old
            try:self.start()
            except Exception:pass
            raise Invalid('代理配置应用失败，已恢复原配置') from None

    def import_source(self,payload):
        name=clean_name(payload.get('name','新订阅'));kind=payload.get('kind')
        content=payload.get('content','')
        if not isinstance(content,str):raise Invalid('导入内容无效')
        if kind not in ('subscription','nodes'):raise Invalid('导入方式无效')
        parsed=parse_nodes(fetch_subscription(content) if kind=='subscription' else content)
        with self.lock:
            self.check_version(payload)
            if len(self.state['sources'])>=20 or len(self.nodes())+len(parsed)>1000:raise Invalid('最多 20 个来源、合计 1000 个节点')
            source_id=uuid.uuid4().hex[:12]
            for n in parsed:n['id']=source_id+'-'+n.pop('key')
            state=copy.deepcopy(self.state)
            state['sources'].append({'id':source_id,'name':name,'kind':kind,'url':content if kind=='subscription' else None,'nodes':parsed,'updated':int(time.time())})
            self.commit(state)

    def change(self,action,payload):
        with self.lock:
            self.check_version(payload);state=copy.deepcopy(self.state)
            if action=='select':
                n=next((n for n in self.nodes() if n['id']==payload.get('node_id')),None)
                if not n:raise Invalid('节点不存在')
                if not n['config'].get('udp',True):raise Invalid('此节点未启用 UDP，不能作为 VoWiFi 出口')
                state.update(selected=n['id'],enabled=True)
            elif action=='pause':state['enabled']=False
            elif action in ('refresh','delete'):
                s=next((s for s in state['sources'] if s['id']==payload.get('source_id')),None)
                if not s:raise Invalid('订阅不存在')
                if action=='refresh':
                    if s['kind']!='subscription':raise Invalid('手动导入的节点没有订阅地址')
                    parsed=parse_nodes(fetch_subscription(s['url']))
                    for n in parsed:n['id']=s['id']+'-'+n.pop('key')
                    if len(self.nodes())-len(s['nodes'])+len(parsed)>1000:raise Invalid('节点总数不能超过 1000')
                    s['nodes']=parsed;s['updated']=int(time.time())
                else:state['sources'].remove(s)
                if state['selected'] not in [n['id'] for n in self.nodes(state)]:
                    if state['enabled']:raise Invalid('操作会移除当前出口，请先暂停或切换节点')
                    state['selected']=None
            else:raise Invalid('操作无效')
            self.commit(state)

    def test(self,node_id):
        with self.lock:
            if node_id not in [n['id'] for n in self.nodes()]:raise Invalid('节点不存在')
            try:
                value=self.control('/proxies/'+quote(node_id,safe='')+'/delay?timeout=6000&url=https%3A%2F%2Fwww.gstatic.com%2Fgenerate_204')
                return {'delay_ms':value.get('delay'),'message':'HTTPS 连接成功；UDP 和 VoWiFi 注册需单独验证'}
            except Exception:raise Invalid('HTTPS 连接测试失败或超时；不能据此判断 UDP') from None
