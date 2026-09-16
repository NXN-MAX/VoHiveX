"""Fixed HTTPS IP checks, with separate direct and SOCKS paths; no host mutations."""
import concurrent.futures
import http.client
import ipaddress
import socket
import ssl
import struct
import time
from pathlib import Path

HOSTS = ('api.ipify.org', 'ipv4.icanhazip.com')

def read_exact(sock, count):
    out = b''
    while len(out) < count:
        part = sock.recv(count-len(out))
        if not part: raise OSError('SOCKS response ended')
        out += part
    return out

def socks_socket(port, host):
    sock = socket.create_connection(('127.0.0.1', port), timeout=6)
    try:
        sock.sendall(b'\x05\x01\x00')
        if read_exact(sock,2) != b'\x05\x00': raise OSError('SOCKS authentication failed')
        name=host.encode('ascii')
        sock.sendall(b'\x05\x01\x00\x03'+bytes([len(name)])+name+struct.pack('!H',443))
        response=read_exact(sock,4)
        if response[:3] != b'\x05\x00\x00': raise OSError('SOCKS connection failed')
        size={1:4,4:16}.get(response[3])
        if response[3]==3: size=read_exact(sock,1)[0]
        if size is None: raise OSError('Invalid SOCKS address')
        read_exact(sock,size+2)
        return sock
    except Exception:
        sock.close(); raise

def nas_source():
    # Refuse cellular default routes and ambiguous routing on the host network.
    import fcntl
    routes=[]
    for line in Path('/proc/net/route').read_text().splitlines()[1:]:
        fields=line.split()
        if len(fields)>7 and fields[1]=='00000000' and int(fields[3],16)&1:
            routes.append((int(fields[6]),fields[0]))
    if not routes: raise OSError('No default route')
    metric=min(r[0] for r in routes)
    names={name for value,name in routes if value==metric}
    if len(names)!=1: raise OSError('Ambiguous default route')
    name=names.pop()
    if name.startswith(('wwan','ppp','usb','rmnet')): raise OSError('Cellular default route')
    with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as sock:
        value=fcntl.ioctl(sock.fileno(),0x8915,struct.pack('256s',name.encode()[:15]))
    return socket.inet_ntoa(value[20:24]),name

def query_ip(port=None):
    source=None; interface=None
    if port is None:
        try: source,interface=nas_source()
        except Exception: return {'ip':None,'error':'无法确认设备默认网络，未执行查询'}
    for host in HOSTS:
        sock=None; conn=None
        try:
            if port is not None: sock=socks_socket(port,host)
            else:
                address=socket.getaddrinfo(host,443,socket.AF_INET,socket.SOCK_STREAM)[0][4]
                if not ipaddress.ip_address(address[0]).is_global: raise OSError('Invalid DNS address')
                sock=socket.create_connection(address,timeout=6,source_address=(source,0))
            sock=ssl.create_default_context().wrap_socket(sock,server_hostname=host)
            conn=http.client.HTTPConnection(host,timeout=6);conn.sock=sock
            conn.request('GET','/',headers={'Host':host,'Connection':'close','User-Agent':'VoHive-IP/1.0'})
            response=conn.getresponse();raw=response.read(257)
            if response.status!=200 or len(raw)>256: raise ValueError('Invalid IP response')
            ip=ipaddress.ip_address(raw.decode('ascii').strip())
            if ip.version!=4 or not ip.is_global: raise ValueError('Not a public IPv4')
            return {'ip':str(ip),'error':None,'checked_at':int(time.time()),'interface':interface}
        except Exception: pass
        finally:
            if conn: conn.close()
            if sock: sock.close()
    return {'ip':None,'error':'查询失败或超时'}

class EgressProbe:
    def __init__(self):
        import threading
        self.lock=threading.Lock();self.cache={}
    def snapshot(self,manager,force=False):
        with manager.lock:
            key=(manager.state['version'],manager.state['selected'],manager.state['enabled'],bool(manager.process and manager.process.poll() is None))
        with self.lock:
            now=time.monotonic()
            if key in self.cache and now-self.cache[key][0]<(5 if force else 60):return self.cache[key][1]
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                nas=pool.submit(query_ip)
                proxy=pool.submit(query_ip,manager.port) if key[2] and key[3] and key[1] else None
                value={'nas':nas.result(),'proxy':proxy.result() if proxy else {'ip':None,'error':'未连接'},'selected':key[1],'version':key[0]}
            with manager.lock:
                current=(manager.state['version'],manager.state['selected'],manager.state['enabled'],bool(manager.process and manager.process.poll() is None))
            if current!=key:
                value['proxy']={'ip':None,'error':'连接状态已变化，请刷新'}
                return value
            self.cache={key:(time.monotonic(),value)}
            return value

def device_egress(devices, rules, proxies, snapshot):
    """Match the SIM home MCC and enabled country/proxy records, not core state alone."""
    result={}
    for device in devices:
        item={'ip':None,'label':'IP 地址','error':'无法确认出口规则'}
        if not device.get('vowifi_enabled'):
            ip = next((device.get(key) for key in ('public_ip','public_ipv6','private_ip','private_ipv6') if device.get(key)),None)
            item={'ip':ip,'label':'IP 地址','error':None if ip else '尚未获取到 SIM 卡 IP'}
        else:
            mcc=str((device.get('modem') or {}).get('native_mcc') or '')
            if mcc and isinstance(rules,list) and isinstance(proxies,list):
                matches=[r for r in rules if r.get('enabled') and mcc in [str(m) for m in r.get('mccs',[])]]
                if len(matches)<=1:
                    proxy=next((p for p in proxies if matches and p.get('id')==matches[0].get('upstream_proxy_id') and p.get('enabled')),None)
                    if proxy is None:item={**snapshot['nas'],'label':'IP 地址'}
                    elif proxy['id']=='vohive-mihomo':item={**snapshot['proxy'],'label':'代理 IP'}
                    else:item={'ip':None,'label':'代理 IP','error':'此代理出口尚未查询'}
        result[device['id']]=item
    return result
