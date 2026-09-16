import {X as h} from '/assets/vendor-DJI26060.js';
import {b as api} from '/assets/route-sms-DJI26060.js';
export default {
 data:()=>({info:null,now:0,error:'',copyStatus:'',clock:null,sync:null,disposed:false}),
 async mounted(){await this.load();if(this.disposed)return;this.clock=setInterval(()=>{if(this.info)this.now=this.info.system_time*1000+(performance.now()-this.received)},1000);this.sync=setInterval(()=>this.load(),60000)},
 beforeUnmount(){this.disposed=true;clearInterval(this.clock);clearInterval(this.sync)},
 methods:{
  async load(){try{const {data}=await api.get('/settings/system');if(this.disposed)return;this.info=data;this.received=performance.now();this.now=data.system_time*1000;this.error=''}catch{if(!this.disposed)this.error='系统信息加载失败，请稍后重试'}},
  format(value){if(!value)return '未记录';return new Date(value).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false})},
  async copy(){const text=this.info?.config_path;if(!text)return;try{
   if(window.isSecureContext&&navigator.clipboard?.writeText)await navigator.clipboard.writeText(text);
   else {const input=document.createElement('textarea');input.value=text;input.readOnly=true;input.style.cssText='position:fixed;left:-9999px;top:0';const focus=document.activeElement;document.body.appendChild(input);try{input.select();if(!document.execCommand('copy'))throw Error('copy failed')}finally{input.remove();focus?.focus()}}
   this.copyStatus='已复制配置文件路径';
  }catch{this.copyStatus='复制失败，请选择路径手动复制'}}
 },
 render(){const row=(label,value)=>h('div',{class:'system-info-row'},[h('dt',null,label),h('dd',null,typeof value==='string'?value:[value])]);return h('div',{class:'system-information'},[
  h('h3',null,'系统信息'),this.error?h('p',{class:'username-error',role:'alert'},this.error):null,
  h('dl',null,[row('系统时间',this.info?this.format(this.now):'加载中…'),row('打包时间',this.info?this.format(this.info.build_time):'加载中…'),row('驱动版本号',this.info?this.info.driver_version||'暂不可用':'加载中…'),row('代理模块版本号',this.info?this.info.proxy_version||'暂不可用':'加载中…'),row('配置文件',this.info?h('button',{type:'button',class:'config-path-copy',onClick:this.copy,'aria-label':'复制配置文件路径'},this.info.config_path):'加载中…'),row('API 文档',h('a',{href:'/api/docs',target:'_blank',rel:'noopener noreferrer'},'查看本机接口说明'))]),
  h('p',{class:'system-copy-status',role:'status'},this.copyStatus)
 ])}
};
