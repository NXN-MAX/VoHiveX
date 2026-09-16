#!/usr/bin/env python3
"""Extract verified customized assets, add a Vue route and sidebar entry."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import xml.etree.ElementTree as ET

root = Path(__file__).resolve().parents[2]
manifest = json.loads((root/'app/patch-manifest.json').read_text())
report = json.loads((root/'release/patch-result.json').read_text())
binary = (root/'release/vohive-dji-amd64').read_bytes()
assert hashlib.sha256(binary).hexdigest() == report['patched_sha256']
version = 'DJI26095'
out = root/'app/scheduler/assets'
if out.exists():
    shutil.rmtree(out)
(out/'assets').mkdir(parents=True, exist_ok=True)
(out/'build-info.json').write_text(json.dumps({'build_time':datetime.now(timezone.utc).isoformat()}))
for asset in manifest['assets']:
    name = asset['name'][5:]
    payload = binary[asset['offset']:asset['offset']+asset['size']].decode()
    if name == 'index.html':
        payload = payload.replace('<title>VoHive</title>','<title>VoHiveX</title>')
        payload = payload.replace('<link rel="icon" type="image/svg+xml" href="/favicon.svg" />','<link rel="icon" type="image/x-icon" href="/assets/vohivex-favicon.ico" />\n  <link rel="apple-touch-icon" href="/assets/vohivex-icon.png" />')
        payload = re.sub(r'  <link[^>]+(?:fonts.googleapis.com|fonts.gstatic.com)[^>]*>\n', '', payload)
        payload = payload.replace('lang="en"','lang="zh-CN"')
        payload = payload.replace('</head>', '<link rel="stylesheet" href="/assets/wise-theme-'+version+'.css">\n</head>')
    if name.endswith('.js'):
        name = re.sub(r'-[^/]{8}\.js$', '-'+version+'.js', name)
    payload = payload.replace('DJI26003',version)
    payload = re.sub(r'#10b981', '#afe67f', payload, flags=re.I)
    payload = payload.replace('16 185 129', '175 230 127').replace('16,185,129', '175,230,127')
    if '/index-' in '/'+name and name.endswith('.js'):
        payload = payload.replace('VoHive 最终用户许可与免责声明','VoHiveX 最终用户许可与免责声明').replace('本软件（VoHive）','本软件（VoHiveX）')
        needle = 'path:"/devices"'
        assert payload.count(needle) == 1
        payload = payload.replace(needle, 'path:"/tasks",name:"ScheduledTasks",component:()=>import("/assets/ScheduledTasks-'+version+'.js"),meta:{requiresAuth:!0}},{'+needle)
        payload = payload.replace('path:"/settings",name:"Settings"', 'path:"/push",name:"MessagePush",component:()=>import("/assets/Settings-'+version+'.js"),props:{pushOnly:true},meta:{requiresAuth:!0}},{path:"/settings",name:"Settings"')
    if 'AuthenticatedShell-' in name:
        needle = r'(\{index:"/sms",label:"短信中心",icon:([^}]+)\}),'
        # Same icon system, with a simple clock glyph rendered by existing Vue.
        payload, count = re.subn(needle, lambda m:m[1]+',{index:"/tasks",label:"定时任务",icon:{render:()=>e("svg",{viewBox:"0 0 24 24",fill:"none",stroke:"currentColor","stroke-width":1.7},[e("circle",{cx:12,cy:12,r:9}),e("path",{d:"M12 7v5l3 2"})])}},',payload)
        assert count == 1
    if name == 'assets/route-proxy-'+version+'.js':
        payload = payload.replace('_("div",_t,[l(et,{title:"代理管理"', '_("div",{..._t,class:_t.class+" proxy-page"},[l(et,{title:"代理管理"')
        # Insert into the existing Element Plus tabs so keyboard navigation and
        # active-tab styling follow the original proxy management page.
        replacements = [
            ('const g=i("upstream"),x=xt()', 'const managedStatus=i(null),g=i("managed"),x=xt()'),
            ('default:n(()=>[l(w,{name:"upstream"}',
             'default:n(()=>[l(w,{name:"managed"},{label:n(()=>[t("span",{class:"inline-flex items-center gap-2"},[t("span",null,"订阅与节点"),managedStatus.value?t("span",{style:"background:#dcfce7;color:#15803d;border-radius:999px;padding:1px 7px;font-size:12px;line-height:20px",title:"节点延时 "+managedStatus.value.actual+"ms"},managedStatus.value.delay+"ms"):null])]),default:n(()=>[l(Managed,{onUpdated:()=>K(),onStatus:value=>managedStatus.value=value})]),_:1}),l(w,{name:"upstream"}'),
            ('async function K(a={})',
             'const builtinPending=i({});async function toggleUpstream(a,enabled){if(builtinPending.value[a.id])return;builtinPending.value[a.id]=true;try{const body=a.id==="vohive-mihomo"?{enabled}:{...a,enabled,password:a.password==="****"?"":a.password};const result=await C.updateProxy(a.id,body);if(!result.ok)throw new Error(result.error.message||"更新失败");await K({silent:true})}catch(error){c.error(error.message||"更新失败");await K({silent:true})}finally{builtinPending.value[a.id]=false}}async function K(a={})'),
            ('l(X,{size:"small",type:s.enabled?"success":"info"},{default:n(()=>[U(m(s.enabled?"已启用":"已禁用"),1)]),_:2},1032,["type"])',
             't("span",{title:"禁用后绑定到该代理的国家规则会回退为直连"},[l(re,{modelValue:s.enabled,"onUpdate:modelValue":value=>toggleUpstream(s,value),loading:builtinPending.value[s.id],disabled:builtinPending.value[s.id],"aria-label":"启用代理 "+(s.name||s.id),"active-text":"已启用","inactive-text":"已禁用","inline-prompt":true},null,8,["modelValue","loading","disabled","aria-label"])])'),
            ('l(fe,null,{default:n(()=>[l(V,{size:"small",onClick:W=>ve(s)}',
             's.id==="vohive-mihomo"?F("",!0):l(fe,null,{default:n(()=>[l(V,{size:"small",onClick:W=>ve(s)}'),
        ]
        replacements += [
            ('device_id:"",enabled:!0,mode:"socks5"', 'device_id:"",enabled:!1,mode:"socks5"'),
            ('||"",enabled:!0,mode:"socks5"', '||"",enabled:!1,mode:"socks5"'),
            ('async function Se(a)', 'async function toggleOutbound(row,enabled){if(G.value)return;G.value=true;try{await setOutboundEnabled(x,row.id,enabled)}catch(error){c.error(error.message||"更新代理失败")}finally{await y({silent:true});G.value=false}}async function Se(a)'),
            ('l(X,{size:"small",type:s.enabled?"success":"info"},{default:n(()=>[U(m(s.enabled?"启用":"禁用"),1)]),_:2},1032,["type"])',
             'l(re,{modelValue:s.enabled,"onUpdate:modelValue":value=>toggleOutbound(s,value),disabled:G.value,loading:G.value,"aria-label":"启用本地出站代理 "+(s.name||s.id),"active-text":"已开启","inactive-text":"已关闭","inline-prompt":true},null,8,["modelValue","disabled","loading","aria-label"])'),
        ]
        replacements += [
            ('v(C).proxies.length>0?(d(),_("span",Ct,m(v(C).proxies.length),1))', 'v(C).proxies.filter(p=>p.enabled).length>0?(d(),_("span",Ct,m(v(C).proxies.filter(p=>p.enabled).length),1))'),
            ('b.value.length>0?(d(),_("span",ht,m(b.value.length),1))', 'b.value.filter(p=>p.enabled).length>0?(d(),_("span",ht,m(b.value.filter(p=>p.enabled).length),1))'),
        ]
        # Remove decorative section heading icons, preserving tab/navigation icons.
        for icon in ['t("div",Ut,[l(r,{size:"20"},{default:n(()=>[l(v(he))]),_:1})]),',
                     't("div",Lt,[l(r,{size:"20"},{default:n(()=>[l(v(Ie))]),_:1})]),']:
            assert payload.count(icon) == 1, icon
            payload = payload.replace(icon, '')
        # Remove the superseded start/stop/restart icon group and form switch.
        start = 'l(fe,{class:"ml-2"},{default:n(()=>['
        end = ',l(V,{size:"small",onClick:B=>de(s)}'
        assert payload.count(start) == payload.count(end) == 1
        left = payload.index(start); right = payload.index(end,left)
        payload = payload[:left]+payload[right+1:]
        start = ',t("div",os,['
        end = ']),t("div",us,['
        assert payload.count(start) == payload.count(end) == 1
        left = payload.index(start); right = payload.index(end,left)
        payload = payload[:left]+payload[right:]
        for before, after in replacements:
            assert payload.count(before) == 1, before
            payload = payload.replace(before, after)
        payload = 'import {setOutboundEnabled} from "./outbound-toggle-'+version+'.js";import Managed from "./ManagedProxy-'+version+'.js";' + payload
        (out/'assets'/('OriginalProxy-'+version+'.js')).write_text(payload)
        payload = 'export {default} from "./OriginalProxy-'+version+'.js";'
    if name == 'assets/route-devices-'+version+'.js':
        payload=payload.replace(' · 公网 IP: ', ' · 模块公网 IP: ')
    if name == 'assets/Dashboard-'+version+'.js':
        # Put the live counts beside the title instead of four standalone cards.
        start = '_(le,{title:"设备监控",subtitle:"实时查看设备状态与出口 IP"}'
        end = ',i(u)?(l(),$(re,'
        assert payload.count(start) == payload.count(end) == 1
        left = payload.index(start); right = payload.index(end, left)
        header = 'e("header",{class:"dashboard-heading"},[_(le,{class:"dashboard-heading-title",title:"设备监控",subtitle:"实时查看设备状态与出口 IP"}),e("dl",{class:"dashboard-summary","aria-label":"设备概况"},[e("div",null,[e("dt",null,"设备总数"),e("dd",null,f(s.value),1)]),e("div",null,[e("dt",null,"在线设备"),e("dd",null,f(n.value),1)]),e("div",null,[e("dt",null,"离线设备"),e("dd",null,f(b.value),1)]),e("div",null,[e("dt",null,"最后刷新"),e("dd",null,f(h.value?new Date(h.value).toLocaleTimeString():"--:--:--"),1)])]),e("div",{class:"dashboard-refresh"},[_(oe,{loading:i(c),onClick:G},null,8,["loading"])])])'
        payload = payload[:left] + header + payload[right:]
        icon = 'e("div",ge,[_(n,{size:"20"},{default:L(()=>[_(i(H))]),_:1})]),'
        assert payload.count(icon) == 1
        payload = payload.replace(icon, '')
        before='e("div",De,[e("span",Se,[_(n,null,{default:L(()=>[_(i(K))]),_:1}),s[1]||(s[1]=se(" 公网 IP",-1))]),e("span",Re,f(a.device.public_ip||"---"),1)])'
        assert payload.count(before)==1
        payload=payload.replace(before,'_(EgressIPs,{compact:true,moduleIP:a.device.public_ip||a.device.public_ipv6||a.device.private_ip||a.device.private_ipv6,deviceId:a.device.id,vowifiEnabled:!!a.device.vowifi_enabled},null,8,["moduleIP","deviceId","vowifiEnabled"])')
        payload='import EgressIPs from "./EgressIPs-'+version+'.js";'+payload
    if name == 'assets/route-sms-'+version+'.js':
        start = 'Hs=le({__name:"RefreshButton"'
        end = ';function Fs('
        assert payload.count(start) == payload.count(end) == 1
        left = payload.index(start); right = payload.index(end, left)
        refresh = 'Hs=le({__name:"RefreshButton",props:{loading:{type:Boolean},disabled:{type:Boolean}},emits:["click"],setup(e){return(a,n)=>(u(),y("button",{type:"button",disabled:e.disabled||e.loading,"aria-busy":e.loading,class:"vh-refresh-button",onClick:n[0]||(n[0]=()=>a.$emit("click"))},e.loading?"刷新中…":"刷新",9,["disabled","aria-busy"]))}})'
        payload = payload[:left] + refresh + payload[right:]
    if name == 'assets/Login-'+version+'.js':
        replacements = [
            ('a=f(!1);async function v(){', 'a=f(!1),showPassword=f(!1),errors=f({username:"",password:"",form:""});async function v(){if(a.value)return;errors.value={username:"",password:"",form:""};'),
            ('if(!r.value.username||!r.value.password){i.warning("请输入用户名和密码");return}', 'errors.value.username=r.value.username.trim()?"":"请填写用户名";errors.value.password=r.value.password?"":"请填写密码";if(errors.value.username||errors.value.password)return;'),
            ('else i.error("登录失败，请检查凭证")', 'else errors.value.form="登录失败，请检查用户名和密码，或稍后重试。"'),
        ]
        for before, after in replacements:
            assert payload.count(before) == 1, before
            payload = payload.replace(before, after)
        start = 'return(i,t)=>(l(),n("div",j,'
        end = '}});export{W as default};'
        assert payload.count(start) == payload.count(end) == 1
        left = payload.index(start); right = payload.index(end, left)
        # Preserve authentication, validation and redirect logic; replace presentation only.
        login = (root/'app/frontend/login-render.js').read_text().strip()
        payload = payload[:left] + login + payload[right:]
        icon_root = root/'app/frontend/remixicon'
        icons = []
        for icon in ['eye-line', 'eye-off-line']:
            svg = ET.fromstring((icon_root/(icon+'.svg')).read_text())
            icons.append(svg.find('{http://www.w3.org/2000/svg}path').attrib['d'])
        payload = 'const eyePath='+json.dumps(icons[0])+',eyeOffPath='+json.dumps(icons[1])+';'+payload
    if name == 'assets/Settings-'+version+'.js':
        before='o("p",{class:"text-sm text-gray-500 mt-2 mb-4"},"查看本机接口说明"),o("a",{href:"/api/docs",target:"_blank",rel:"noopener noreferrer",class:"inline-flex px-4 py-2 rounded-lg bg-indigo-600 text-white font-medium"},"打开 API 文档")'
        after='o("a",{href:"/api/docs",target:"_blank",rel:"noopener noreferrer",class:"inline-block text-sm text-indigo-600 hover:underline mt-2"},"查看本机接口说明")'
        assert payload.count(before)==1
        payload=payload.replace(before,after)
    (out/name).write_text(payload)
for source, target in [('Tasks','ScheduledTasks'),('ManagedProxy','ManagedProxy'),('qr-import','qr-import'),('outbound-toggle','outbound-toggle'),('EgressIPs','EgressIPs')]:
    text=(root/'app/scheduler'/(source+'.js')).read_text()
    text=re.sub(r'DJI260\d{2}',version,text)
    subprocess.run(['node',str(root/'app/build-tools/node_modules/terser/bin/terser'),'--module','--compress','--mangle','-o',str(out/'assets'/(target+'-'+version+'.js'))],input=text,text=True,check=True)
(out/'assets'/('ScheduledTasks-'+version+'.css')).write_text((root/'app/scheduler/Tasks.css').read_text())
(out/'assets'/('ManagedProxy-'+version+'.css')).write_text((root/'app/scheduler/ManagedProxy.css').read_text())
for branding_file in ['Jost-Italic.ttf','vohivex-favicon.ico','vohivex-icon.png']:
    shutil.copyfile(root/'app/branding'/branding_file,out/'assets'/branding_file)
shutil.copyfile(root/'app/branding/OFL.txt',out/'assets/Jost-OFL.txt')
shutil.copyfile(root/'app/build-tools/node_modules/jsqr/dist/jsQR.js',out/'assets'/('jsQR-'+version+'.js'))
shutil.copyfile(root/'app/frontend/wise-theme.css',out/'assets'/('wise-theme-'+version+'.css'))
for component in ['UsernameSettings','SystemInformation','VersionInformation']:
    text=re.sub(r'DJI260\d{2}',version,(root/'app/frontend'/(component+'.js')).read_text())
    subprocess.run(['node',str(root/'app/build-tools/node_modules/terser/bin/terser'),'--module','--compress','--mangle','-o',str(out/'assets'/(component+'-'+version+'.js'))],input=text,text=True,check=True)
# Versioned, locally served Swagger resources; no CDN or validator requests.
docs = out/'api/docs'
docs.mkdir(parents=True, exist_ok=True)
shutil.copyfile(root/'app/docs/index.html',docs/'index.html')
shutil.copytree(root/'app/docs/vendor/swagger-ui-5.32.15',docs/'assets/5.32.15')
shutil.copyfile(root/'app/frontend/remixicon/LICENSE',out/'assets/remixicon-LICENSE.txt')
import runpy
runpy.run_path(str(root/'app/frontend/remix-ui.py'))['apply'](root,out,version)
print('Prepared',len(list(out.rglob('*.*'))),'assets')
