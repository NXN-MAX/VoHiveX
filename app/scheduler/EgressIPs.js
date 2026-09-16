import { X as h } from '/assets/vendor-DJI26006.js';
export default {
 name:'EgressIPs',props:['connectionKey','compact','moduleIP','deviceId','vowifiEnabled'],
 data(){return{ips:null,busy:false,error:'',copyStatus:'',generation:0};},
 mounted(){this._alive=true;this._abort=new AbortController();this.refresh();this._timer=setInterval(()=>this.refresh(),60000);},
 beforeUnmount(){this._alive=false;clearInterval(this._timer);this._abort.abort();},
 watch:{connectionKey(){this.ips=null;this.generation++;this.refresh();},vowifiEnabled(){this.ips=null;this.generation++;this.refresh();},deviceId(){this.ips=null;this.generation++;this.refresh();}},
 methods:{async copyIP(ip){if(!ip)return;try{
  if(window.isSecureContext&&navigator.clipboard?.writeText)await navigator.clipboard.writeText(ip);
  else {const input=document.createElement('textarea');input.value=ip;input.readOnly=true;input.style.cssText='position:fixed;left:-9999px;top:0';const focus=document.activeElement;document.body.appendChild(input);try{input.select();if(!document.execCommand('copy'))throw Error('copy failed')}finally{input.remove();focus?.focus()}}
  this.copyStatus='IP 已复制';
 }catch{this.copyStatus='复制失败，请选择 IP 手动复制'}},async refresh(force=false){const generation=++this.generation;if(this.compact&&!this.vowifiEnabled){this.ips=null;this.busy=false;this.error='';return;}this.busy=true;this.error='';try{const response=await fetch('/api/managed-proxy/public-ip'+(force?'?refresh=1':''),{headers:{Authorization:'Bearer '+(localStorage.getItem('token')||'')},signal:this._abort.signal,cache:'no-store'});const data=await response.json();if(!response.ok)throw Error(data.error||'查询失败');if(this._alive&&generation===this.generation)this.ips=data;}catch(e){if(this._alive&&generation===this.generation){this.ips=null;this.error=e.message;}}finally{if(this._alive&&generation===this.generation)this.busy=false;}}},
 render(){if(this.compact){const entry=this.vowifiEnabled?this.ips?.devices?.[this.deviceId]:null;const ip=this.vowifiEnabled?entry?.ip:this.moduleIP;const label=entry?.label==='代理 IP'?'代理 IP':'IP 地址';return h('div',{class:'dashboard-ip-row'},[h('span',{class:'dashboard-ip-label'},label),h('span',{class:'dashboard-ip-value',title:ip||this.error||entry?.error||'尚未获取到 SIM 卡 IP'},ip||(this.busy?'查询中…':'暂不可用'))]);}return h('div',{class:'egress-ip-block'},[h('section',{class:'egress-ips','aria-label':'公网 IP'},[
  ...[['设备公网IP','nas'],['订阅代理出口 IP','proxy']].map(([label,key])=>{
   const ip=this.ips?.[key]?.ip;const text=ip||this.ips?.[key]?.error||(this.busy?'查询中…':this.error||'未查询');
   return h('div',{class:'egress-ip-item'},[h('span',{class:'egress-ip-label'},label),ip?h('button',{type:'button',class:'egress-ip-value',title:ip,'aria-label':'复制'+label,onClick:()=>this.copyIP(ip)},ip):h('span',{class:'egress-ip-empty',title:text},text)]);
  }),
  h('button',{type:'button',class:'mp-button egress-refresh',disabled:this.busy,onClick:()=>{this.copyStatus='';this.refresh(true)}},this.busy?'查询中…':'刷新')
 ]),h('p',{class:'egress-copy-status',role:'status'},this.copyStatus)]);}
};
