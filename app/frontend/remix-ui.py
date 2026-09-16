"""Apply the scoped Remix Icon presentation to generated Vue chunks."""
import json
import xml.etree.ElementTree as ET
from urllib.parse import quote
from pathlib import Path


def apply(root, out, version):
    icons = {'sortAsc':'sort-asc','sortDesc':'sort-desc','cellular1':'signal-cellular-1-fill','cellular2':'signal-cellular-2-fill','cellular3':'signal-cellular-3-fill','cellularOff':'signal-cellular-off-line','notify':'notification-3-line','delete':'delete-bin-line','dashboard':'dashboard-line','device':'smartphone-line','global':'global-line','sms':'message-2-line','tasks':'calendar-schedule-line','logs':'file-list-3-line','settings':'settings-3-line','add':'add-line','router':'router-line','wifi':'wifi-line','user':'ghost-line','close':'close-line','fold':'menu-fold-line','unfold':'menu-unfold-line','logout':'logout-box-r-line','sun':'sun-line','moon':'moon-line','sim':'sim-card-line','terminal':'terminal-box-line','ussd':'chat-voice-line','policy':'shield-check-line'}
    icon_map=json.loads((root/'app/frontend/remix-map.json').read_text())
    for family in icon_map.values(): icons.update(family)
    icons["empty"]="inbox-line"
    paths = {}
    for key, name in icons.items():
        svg = ET.fromstring((root/'app/frontend/remixicon'/(name+'.svg')).read_text())
        paths[key] = [p.attrib['d'] for p in svg.findall('{http://www.w3.org/2000/svg}path')]
    module = 'import {X as h,V as v} from "./vendor-'+version+'.js";const paths='+json.dumps(paths)+';export const Remix=Object.fromEntries(Object.entries(paths).map(([name,paths])=>[name,{render:()=>h("svg",{viewBox:"0 0 24 24",fill:"currentColor","aria-hidden":"true",class:"ri-icon ri-"+name},paths.map(d=>h("path",{d})))}]));'
    module += 'export const IconButton={props:["icon","label"],emits:["click"],setup:(p,{emit})=>()=>h("button",{type:"button",class:"vh-icon-button","aria-label":p.label,title:p.label,onClick:()=>emit("click")},[v(p.icon)])};'
    (out/'assets'/('RemixIcons-'+version+'.js')).write_text(module)

    def replace(s, old, new, count=1):
        assert s.count(old)==count, (old, s.count(old))
        return s.replace(old,new)
    for filename in ['AuthenticatedShell','OriginalProxy','Dashboard','route-devices','route-sms','route-logs','Settings','SwitchDark.vue_vue_type_script_setup_true_lang']:
        path = out/'assets'/(filename+'-'+version+'.js')
        s = path.read_text()
        if filename=='AuthenticatedShell':
            s=replace(s,'b("div",{key:p.value},[_e(l.$slots,"default")])','b("div",{key:p.value,class:"vh-module-grid"},[_e(l.$slots,"default")])')
            for label, old, new in [('仪表盘','he','dashboard'),('设备管理','ke','device'),('代理管理','Ee','global'),('短信中心','Ve','sms'),('实时日志','Ae','logs'),('系统设置','C','settings')]:
                s=replace(s,'label:"'+label+'",icon:'+old,'label:"'+label+'",icon:Remix.'+new)
            clock='{render:()=>e("svg",{viewBox:"0 0 24 24",fill:"none",stroke:"currentColor","stroke-width":1.7},[e("circle",{cx:12,cy:12,r:9}),e("path",{d:"M12 7v5l3 2"})])}'
            s=replace(s,clock,'Remix.tasks')
            s=replace(s,'s[4]||(s[4]=e("div",{class:"sidebar-brand-icon"},"V",-1)),','')
            s=replace(s,'Ce={key:0,class:"ml-3"}','Ce={key:0,class:"sidebar-wordmark"}')
            s=replace(s,'e("div",{class:"sidebar-brand-icon"},"V"),e("div",{class:"ml-3"},','e("div",{class:"sidebar-wordmark"},')
            s=replace(s,'t(u(C))','t(Remix.user)',2)
            for key in ['De','Oe']:
                old=key+'={class:"w-9 h-9 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-300"}'
                s=replace(s,old,key+'={class:"sidebar-user-ghost"}')
            s=replace(s,'t(V,{text:"",type:"danger",onClick:S},{default:a(()=>[t(d,null,{default:a(()=>[t(u(z))]),_:1})]),_:1})','t(IconButton,{icon:Remix.logout,label:"退出登录",onClick:S})',2)
            s=replace(s,'t(V,{text:"",onClick:N,class:"!px-2"},{default:a(()=>[t(d,null,{default:a(()=>[!v.value&&!l.value?(o(),r(u(de),{key:0})):(o(),r(u(ue),{key:1}))]),_:1})]),_:1})','t(IconButton,{icon:!v.value&&!l.value?Remix.fold:Remix.unfold,label:!v.value&&!l.value?"收起导航":"展开导航",onClick:N},null,8,["icon","label"])')
            s=replace(s,'width:l.value?"52px":"232px"','width:l.value?"72px":"232px"')
            s=replace(s,'e("div",je,[t(ye,','e("span",{class:"vh-mobile-wordmark"},"VoHive"),e("div",je,[t(ye,')
            s=replace(s,'{class:"sidebar-brand-title"},"VoHive"','{class:"sidebar-brand-title"},[e("span",null,"VoHive"),e("span",{class:"vh-brand-x"},"X")]',2)
            s=replace(s,'{class:"vh-mobile-wordmark"},"VoHive"','{class:"vh-mobile-wordmark"},[e("span",null,"VoHive"),e("span",{class:"vh-brand-x"},"X")]')
            s=replace(s,'{index:"/logs",label:"实时日志",icon:Remix.logs}', '{index:"/push",label:"消息推送",icon:Remix.notify},{index:"/logs",label:"实时日志",icon:Remix.logs}')
        elif filename=='OriginalProxy':
            for old,new in [('he','global'),('Ie','router')]:
                s=replace(s,'l(r,{size:"16"},{default:n(()=>[l(v('+old+'))]),_:1})','l(r,{size:"16"},{default:n(()=>[l(Remix.'+new+')]),_:1})')
            for old,label in [('Ee','编辑'),('Ue','删除')]:
                s=replace(s,'l(r,null,{default:n(()=>[l(v('+old+'))]),_:1})','t("span",null,"'+label+'")',2)
            # Use the same independent action buttons for both proxy lists.
            upstream='l(V,{size:"small",onClick:W=>ve(s)},{default:n(()=>[t("span",null,"编辑")]),_:1},8,["onClick"]),l(V,{size:"small",type:"danger",onClick:W=>Be(s)},{default:n(()=>[t("span",null,"删除")]),_:1},8,["onClick"])'
            outbound='l(V,{size:"small",onClick:B=>de(s)},{default:n(()=>[t("span",null,"编辑")]),_:1},8,["onClick"]),l(V,{size:"small",type:"danger",onClick:B=>Te(s.id)},{default:n(()=>[t("span",null,"删除")]),_:1},8,["onClick"])'
            s=replace(s,'l(fe,null,{default:n(()=>['+upstream+']),_:2},1024)','t("div",{class:"vh-proxy-row-actions"},['+upstream+'])')
            s=replace(s,outbound,'t("div",{class:"vh-proxy-row-actions"},['+outbound+'])')
        elif filename=='Dashboard':
            s=replace(s,'return Q;const t=c.value','return Remix.wifi;const t=c.value')
            s=replace(s,'group relative block w-full overflow-hidden ui-card ui-card-hover text-left','dashboard-device-card group relative block w-full overflow-hidden ui-card ui-card-hover text-left')
            # SIM silhouette is presentation only: keep existing healthy/VoWiFi branches.
            for old,new in [
                ('ue={class:"p-6 relative z-10"}', 'ue={class:"sim-card-content"}'),
                ('ve={class:"flex justify-between items-start mb-6"}', 've={class:"sim-card-heading"}'),
                ('fe={class:"flex items-center gap-3"}', 'fe={class:"sim-card-name"}'),
                ('me={class:"font-bold text-base text-gray-800 dark:text-gray-100"}', 'me={class:"sim-card-title"}'),
                ('ye={class:"space-y-4"}', 'ye={class:"sim-card-details"}'),
                ('he={class:"flex items-center justify-between p-3 bg-gray-50/50 dark:bg-white/5 rounded-xl border border-gray-100 dark:border-white/5"}', 'he={class:"sim-card-network"}'),
                ('Ce={class:"text-xs font-mono text-gray-400 ml-1 hidden xl:inline"}', 'Ce={class:"sim-card-dbm"}'),
                ('class:"grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-5"', 'class:"sim-card-grid"'),
                ('s[2]||(s[2]=e("div",{class:"absolute top-0 right-0 w-32 h-32 bg-gradient-to-br from-indigo-500/10 to-indigo-400/10 rounded-bl-full -mr-8 -mt-8 transition-transform group-hover:scale-150"},null,-1))', 'e("span",{class:"sim-card-chip","aria-hidden":"true"},[e("span")])'),
                ('e("h3",me,f(a.device.name||a.device.id),1)', 'e("h3",{...me,title:a.device.name||a.device.id},f(a.device.name||a.device.id),9,["title"])'),
                ('f(a.device.signal_dbm)+"dBm"', 'C(a.device.signal_dbm)?f(a.device.signal_dbm)+" dBm":"-- dBm"'),
            ]: s=replace(s,old,new)
            bars='e("div",we,[(l(),w(q,null,B(4,b=>e("div",{key:b,class:R(["w-1 rounded-sm transition-all duration-500",h(a.device.signal_dbm)>=b?y(a.device.signal_dbm):"bg-gray-200 dark:bg-gray-700"]),style:te({height:`${b*25}%`})},null,6)),64))])'
            s=replace(s,bars,'e("span",{class:"sim-card-signal","aria-label":C(a.device.signal_dbm)?"蜂窝信号 "+a.device.signal_dbm+" dBm":"蜂窝信号未知"},[_(h(a.device.signal_dbm)===0?Remix.cellularOff:h(a.device.signal_dbm)<=1?Remix.cellular1:h(a.device.signal_dbm)<=2?Remix.cellular2:Remix.cellular3)],8,["aria-label"])')

        elif filename=='route-sms':
            s=replace(s,'o("span",{class:"text-xl font-bold"},"∅",-1)','o("span",{class:"text-xl"},[d(Remix.empty)])')
            s=replace(s,'Ts={class:"flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 sm:mb-8"}','Ts={class:"vh-page-heading"}')
            s=replace(s,'$s={class:"text-3xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-gray-900 to-gray-600 dark:from-white dark:to-gray-400"}','$s={class:"vh-page-title"}')
            s=replace(s,'Cs={key:0,class:"text-gray-500 dark:text-gray-400 mt-1"}','Cs={key:0,class:"vh-page-subtitle"}')
            s=replace(s,':k("",!0)]),Ne(a.$slots,"actions")',':k("",!0)]),o("div",{class:"vh-page-actions"},[Ne(a.$slots,"actions")])')
            s=replace(s,'onClick:Ie(w=>void dt(r),["stop"])},{default:m(()=>[d(i,null,{default:m(()=>[d(I($e))]),_:1})])','onClick:Ie(w=>void dt(r),["stop"])},{default:m(()=>[d(Remix.delete)])')
            s=replace(s,'Gs={class:"flex-1 ui-card overflow-hidden relative"}','Gs={class:"sms-content-panel flex-1 ui-card overflow-hidden relative"}')
            s=replace(s,'Ba={class:"flex items-end gap-3"}','Ba={class:"sms-composer flex items-end gap-3"}')
            s=replace(s,'px-5 py-4 rounded-2xl text-sm leading-[1.75] shadow-sm border','sms-message-bubble px-5 py-4 rounded-2xl text-sm leading-[1.75] border')
        elif filename=='route-devices':
            # Keep the original filter/sort models and replace only their controls.
            start='t("div",fs,[r(R,{modelValue:b.value'
            end=',e.deviceLimit>0?'
            assert s.count(start)==s.count(end)==1
            left=s.index(start);right=s.index(end,left)
            controls='t("div",{class:"vh-device-filters"},[t("div",{class:"vh-device-status-options",role:"radiogroup","aria-label":"设备状态"},[t("label",null,[t("input",{type:"radio",name:"device-status",value:"all",checked:b.value==="all",onChange:()=>b.value="all"},null,40,["checked","onChange"]),t("span",null,"全部")]),t("label",null,[t("input",{type:"radio",name:"device-status",value:"online",checked:b.value==="online",onChange:()=>b.value="online"},null,40,["checked","onChange"]),t("span",null,"在线")]),t("label",null,[t("input",{type:"radio",name:"device-status",value:"offline",checked:b.value==="offline",onChange:()=>b.value="offline"},null,40,["checked","onChange"]),t("span",null,"离线")])]),t("div",{class:"vh-device-sort"},[t("button",{type:"button",class:"vh-sort-key","aria-label":g.value==="name"?"按名称排序，点击切换为信号":"按信号排序，点击切换为名称",onClick:()=>g.value=g.value==="name"?"signal":"name"},g.value==="name"?"名称":"信号",9,["aria-label","onClick"]),t("button",{type:"button",class:"vh-sort-direction","aria-label":I.value==="asc"?"升序，点击切换为降序":"降序，点击切换为升序",onClick:()=>I.value=I.value==="asc"?"desc":"asc"},[r(I.value==="asc"?Remix.sortAsc:Remix.sortDesc)],8,["aria-label","onClick"])])'
            s=s[:left]+controls+s[right:]
            s=replace(s,'ms={key:0,class:"flex items-center"}','ms={key:0,class:"vh-device-quota"}')
            s=replace(s,'vs={class:"flex items-center gap-3 mb-4"}','vs={class:"device-search-row flex items-center gap-3 mb-4"}')
            s=replace(s,'label:"最后原因"','label:"最近原因"')
            cellular='E.value===0?Remix.cellularOff:E.value<=2?Remix.cellular1:E.value<=4?Remix.cellular2:Remix.cellular3'
            s=replace(s,'r(sa,{tone:re.value,size:"sm",animated:H.value},null,8,["tone","animated"])','t("span",{class:fe(["device-cellular-icon",f.value]),role:"img","aria-label":E.value?"蜂窝信号 "+E.value+"/5":"蜂窝信号未知"},[r('+cellular+')],10,["aria-label"])')
            bars='t("div",Tl,[(i(),m(ie,null,De(5,Ce=>t("div",{key:Ce,class:fe(["w-1.5 rounded-sm",Ce<=E.value?S.value:"bg-gray-200 dark:bg-white/10"]),style:gn({height:Ce*18+10+"%"})},null,6)),64))])'
            s=replace(s,bars,'t("span",{class:fe(["device-cellular-icon device-cellular-strength",f.value]),role:"img","aria-label":E.value?"蜂窝信号 "+E.value+"/5":"蜂窝信号未知"},[r('+cellular+')],10,["aria-label"])')
            s=replace(s,'l[7]||(l[7]=t("div",{class:"device-header-brand-icon"},"V",-1)),','')
            s=replace(s,'cs={class:"ui-card p-5"}','cs={class:"ui-card p-5 device-list-panel"}')
            s=replace(s,'_d={class:"ui-card p-6"}','_d={class:"ui-card p-6 device-detail-panel"}')
            s=replace(s,'hs={class:"flex items-center gap-2"}','hs={class:"flex items-center gap-2 vh-device-state"}')
            s=replace(s,'"w-full h-full text-left p-3 rounded-xl border transition-all",e.selectedId===E.id','"vh-device-option w-full h-full text-left p-3 rounded-xl border transition-all",e.selectedId===E.id')
            s=replace(s,'x[12]||(x[12]=t("div",{class:"w-9 h-9 rounded-xl bg-gradient-to-br from-[#5b5bd6] to-[#4a4ac2] text-white text-xs font-bold flex items-center justify-center shadow-lg shadow-indigo-500/25"}," ESIM ",-1))','t("div",{class:"device-tool-icon"},[r(Remix.sim)])')
            for key,old,new in [('ui','Mn','terminal'),('Vi','Rn','ussd'),('po','Ln','policy'),('to','Nn','settings')]:
                import re
                s,count=re.subn(key+r'=\{class:"w-10 h-10 rounded-xl [^"]+"\}',key+'={class:"device-tool-icon"}',s)
                assert count==1,(key,count)
                s=replace(s,'r(h('+old+'))','r(Remix.'+new+')')
            s=replace(s,'Yr={class:"w-7 h-7 rounded-lg bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-400"}','Yr={class:"device-tool-icon"}')
            s=replace(s,'t("div",Yr,[r(Z,{size:"16"},{default:$(()=>[r(h(Ta))]),_:1})])','t("div",Yr,[r(Z,{size:"16"},{default:$(()=>[r(Remix.add)]),_:1})])')
            s=replace(s,'r(h(Ta))]),_:1}),d[8]','r(Remix.add)]),_:1}),d[8]')
            for old,new in [
                ('h(lr)(u.value)?(i(),ae(Z,{key:0,size:"18"},{default:$(()=>[r(h(kt))]),_:1})):B("",!0)', 't("span",null,"刷新")'),
                ('h(sr)(C.value)?(i(),ae(Z,{key:0,size:"18"},{default:$(()=>[r(h(Dn))]),_:1})):B("",!0)', 't("span",null,"当前通知")'),
                ('r(Z,{size:"18"},{default:$(()=>[h(P)?(i(),ae(h(Ca),{key:0})):(i(),ae(h(Sa),{key:1}))]),_:1})', 't("span",null,h(P)?"隐藏敏感信息":"显示敏感信息",1)')]:
                s=replace(s,old,new)
            s=s.replace('circle:"",text:"",','text:"",')
            s=replace(s,'cr={key:0,class:"ui-panel-muted p-4 relative"}','cr={key:0,class:"esim-section-heading"}')
            s=replace(s,'vr={class:"flex items-center justify-between gap-3 mb-3"}','vr={class:"flex items-center justify-between gap-3 esim-heading-row"}')
            s=replace(s,'yr={class:"flex items-center gap-2"}','yr={class:"flex items-center gap-2 device-outline-actions"}')
            s=replace(s,'Bs={class:"flex flex-wrap items-center gap-2"}','Bs={class:"flex flex-wrap items-center gap-2 device-outline-actions"}')
            for label in ['"手动刷新"','"当前通知"','h(P)?"隐藏敏感信息":"显示敏感信息"']:
                s=replace(s,'r(xe,{content:'+label+',placement:"top"}', 'r(xe,{disabled:true,content:'+label+',placement:"top"}')

        elif filename=='route-logs':
            s=replace(s,'S=i(!0),m=i("all")','S=i(!0),wrapLogs=i(false),m=i("all")')
            start='actions:y(()=>[l("div",_e,['
            end=']),_:1}),l("div",xe,'
            assert s.count(start)==s.count(end)==1
            left=s.index(start);right=s.index(end,left)
            actions='actions:y(()=>[l("div",_e,[a(g,{onClick:O,text:true,class:"vh-log-plain"},{default:y(()=>[D("清空")]),_:1}),a(g,{onClick:M,text:true,class:"vh-log-plain"},{default:y(()=>[D("导出")]),_:1}),a(g,{onClick:N,type:u.value?"success":"warning",class:"!border-0"},{default:y(()=>[D(u.value?"继续":"暂停",1)]),_:1},8,["type"])])'
            s=s[:left]+actions+s[right:]
            old='label:"自动追尾"},null,8,["modelValue"])'
            new=old+',a(v,{modelValue:wrapLogs.value,"onUpdate:modelValue":value=>wrapLogs.value=value,label:"自动换行"},null,8,["modelValue"])])'
            s=replace(s,old,new)
            s=replace(s,'a(v,{modelValue:S.value','l("div",{class:"vh-log-options"},[a(v,{modelValue:S.value')
            s=replace(s,'h("span",De,r(s.fields),1)):R("",!0)]))),128))','h("span",De,r(s.fields),1)):R("",!0)],4))),128))')
            s=replace(s,'xe={class:"flex items-center gap-4 mb-4"}','xe={class:"flex flex-wrap items-center gap-4 mb-4"}')
            s=replace(s,'class:"py-0.5 hover:bg-white/5 px-2 -mx-2 rounded whitespace-nowrap"','class:"vh-log-line py-0.5 hover:bg-white/5 px-2 -mx-2 rounded",style:wrapLogs.value?{whiteSpace:"pre-wrap",overflowWrap:"anywhere"}:{whiteSpace:"pre"}')
            s=replace(s,'class:"h-[60vh] overflow-auto font-mono text-sm bg-gray-900 dark:bg-black text-gray-100 p-4"','class:"vh-log-content h-[60vh] overflow-auto font-mono bg-gray-900 dark:bg-black text-gray-100 p-4"')
        elif filename=='Settings':
            s=replace(s,'r(l,null,{default:i(()=>[r(n(Q))]),_:1})','o("span",null,"删除")',3)
            s=replace(s,'__name:"Settings",setup(a){','__name:"Settings",props:{pushOnly:Boolean},setup(settingsProps){')
            s=replace(s,'s(()=>{ee()})','l(()=>settingsProps.pushOnly,value=>{if(value)ee()},{immediate:true})')
            s=replace(s,'d("div",le,[r(w,{title:"系统设置",subtitle:"管理网关参数与运行信息"})','d("div",{class:settingsProps.pushOnly?"push-page":"settings-page"},[r(w,{title:settingsProps.pushOnly?"消息推送":"系统设置",subtitle:settingsProps.pushOnly?"管理消息推送渠道与通知内容":"管理访问凭证与本机接口"})')
            s=replace(s,'o("div",se,[o("div",te,','o("div",{class:settingsProps.pushOnly?"push-layout":"settings-layout"},[settingsProps.pushOnly?u("",true):o("section",{class:"ui-card settings-account-card"},[r(UsernameSettings)]),o("div",te,')
            s=replace(s,'te={class:"ui-card p-8 relative overflow-hidden group"}','te={class:"ui-card settings-password-card p-8 relative overflow-hidden group"}')
            s=replace(s,'pe={class:"ui-card p-8 relative overflow-hidden group"}','pe={class:"ui-card settings-api-card p-8 relative overflow-hidden group"}')
            s=replace(s,'},"安全")','},"密码修改")')
            s=replace(s,'o("div",re,[r(l,{size:"24"},{default:i(()=>[r(n(T))]),_:1})]),','')
            heading='o("div",ge,[o("div",ve,[r(l,{size:"24"},{default:i(()=>[r(n(L))]),_:1})]),a[52]||(a[52]=o("div",null,[o("h3",{class:"text-lg font-bold text-gray-800 dark:text-gray-100"},"通知"),o("p",{class:"text-xs text-gray-500"},"Telegram / 飞书 / QQ / Webhook")],-1))]),'
            s=replace(s,heading,'')
            s=replace(s,' 保存通知配置 ','保存推送配置')
            # Place the existing save action in the shared header, preserving its handler.
            start=s.index('o("div",be,[');end=s.index(',n(y)?',start)
            action=s[start:end]
            s=s[:start]+s[end+1:]
            header='subtitle:settingsProps.pushOnly?"管理消息推送渠道与通知内容":"管理访问凭证与本机接口"})'
            s=replace(s,header,header[:-1]+',{actions:i(()=>settingsProps.pushOnly?['+action+']:[]),_:1})')
            s='import VersionInformation from "./VersionInformation-'+version+'.js";import SystemInformation from "./SystemInformation-'+version+'.js";import UsernameSettings from "./UsernameSettings-'+version+'.js";'+s
        else:
            s='import {X as h,V as v} from "./vendor-'+version+'.js";import {Remix} from "./RemixIcons-'+version+'.js";export const _={props:{isDark:Boolean},emits:["toggle"],setup:(p,{emit})=>()=>h("button",{type:"button",role:"switch",class:"vh-appearance-switch","aria-label":"深色模式","aria-checked":p.isDark,title:p.isDark?"切换为浅色模式":"切换为深色模式",onClick:()=>emit("toggle",!p.isDark)},[h("span",{class:"vh-appearance-thumb"},[v(p.isDark?Remix.moon:Remix.sun)])])};'
        if filename!='SwitchDark.vue_vue_type_script_setup_true_lang':
            s='import {Remix,IconButton} from "./RemixIcons-'+version+'.js";'+s
        path.write_text(s)
        if filename=='Settings':
            import subprocess
            subprocess.run(['node',str(root/'app/frontend/split-settings.cjs'),str(path)],check=True)
    # Native library dialog controls retain their existing close handler and label.
    svg=(root/'app/frontend/remixicon/close-line.svg').read_text()
    css=out/'assets'/('wise-theme-'+version+'.css')
    with css.open('a') as f:
        f.write('\n:root{--vh-close-icon:url("data:image/svg+xml,'+quote(svg)+'")}\n')
        check=(root/'app/frontend/remixicon/check-line.svg').read_text()
        f.write('\n:root{--vh-check-icon:url("data:image/svg+xml,'+quote(check)+'")}\n')

    import subprocess
    for family in icon_map:
        subprocess.run(['node',str(root/'app/frontend/replace-icons.cjs'),str(out/'assets'/(family+'-'+version+'.js')),family,version],check=True)
