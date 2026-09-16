import EgressIPs from '/assets/EgressIPs-DJI26006.js';
import { X as h, V as componentVNode } from '/assets/vendor-DJI26006.js';
import {decodeQrFile} from '/assets/qr-import-DJI26006.js';
import '/assets/jsQR-DJI26006.js';
const format=t=>new Date(t*1000).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false});
export default {
 name:'ManagedProxy',emits:['updated','status'],
 data(){return{state:{sources:[],nodes:[],version:0},loading:true,sourceCollapsed:{},busy:false,error:'',notice:'',form:null,latency:null,latencyNode:null,latencyBusy:false,decodeBusy:false,confirmAction:null,results:{},testId:'',batch:null,focusReturn:null};},
 mounted(){this._alive=true;this._abort=new AbortController();this.refresh();this._poll=setInterval(()=>{if(!this.busy&&!this.batch&&!this.testId)this.refresh();},30000);},
 beforeUnmount(){this._alive=false;clearInterval(this._poll);if(this.batch)this.batch.cancelled=true;this._abort.abort();this.clearForm();},
 methods:{
  async request(action='',body){const r=await fetch('/api/managed-proxy'+(action?'/'+action:''),{method:body?'POST':'GET',signal:this._abort.signal,headers:{Authorization:'Bearer '+(localStorage.getItem('token')||''),'Content-Type':'application/json'},cache:'no-store',body:body?JSON.stringify(body):undefined});const v=await r.json();if(!r.ok)throw Error(v.error||'操作失败');return v;},
  async refresh(){try{const s=await this.request();if(this._alive){this.state=s;this.publishStatus();await this.measureLatency();}}catch(e){if(this._alive)this.error=e.message;}finally{this.loading=false;}},
  publishStatus(){const connected=!!(this.state.enabled&&this.state.running&&this.state.selected);if(!connected||this.latencyNode!==this.state.selected){this.latency=null;this.latencyNode=null;}this.$emit('status',connected&&Number.isFinite(this.latency)?{delay:Math.min(999,Math.max(0,Math.round(this.latency))),actual:this.latency}:null);},
  async measureLatency(){if(this.latencyBusy||!this.state.enabled||!this.state.running||!this.state.selected)return;const id=this.state.selected;this.latencyBusy=true;try{const r=await this.request('test',{node_id:id});if(this._alive&&this.state.selected===id&&this.state.enabled){this.latency=Number.isFinite(r.delay_ms)&&r.delay_ms>=0?r.delay_ms:null;this.latencyNode=id;}}catch{if(this._alive&&this.state.selected===id)this.latency=null;}finally{this.latencyBusy=false;if(this._alive)this.publishStatus();}},
  connect(){const node=this.state.nodes.find(n=>n.id===this.state.selected);if(node)return this.mutate('select',{node_id:node.id});this.error='请先选择一个节点';},
  button(text,fn,primary=false,props={}){return h('button',{type:'button',class:'mp-button'+(primary?' primary':''),disabled:this.busy||this.loading||!!this.batch||!!this.testId,onClick:fn,...props},text);},
  toggleSource(id){this.sourceCollapsed[id]=this.sourceCollapsed[id]===false;},
  open(){this.focusReturn=document.activeElement;this.error='';this.form={name:'新订阅',kind:'subscription',content:''};this.$nextTick(()=>this.$refs.name?.focus());},
  clearForm(){if(this.form)this.form.content='';if(this.$refs.image)this.$refs.image.value='';this.form=null;this.confirmAction=null;},
  close(){if(this.busy||this.decodeBusy)return;this.clearForm();this.$nextTick(()=>this.focusReturn?.focus());},
  async decode(e){this.decodeBusy=true;this.error='';const form=this.form;try{const text=await decodeQrFile(e.target,globalThis.jsQR);if(!this._alive||this.form!==form)return;form.kind=/^https:\/\//i.test(text)?'subscription':'nodes';form.content=text;this.notice='二维码已识别，图片已丢弃；点击“确认导入”后保存识别内容';}catch(e){if(this._alive)this.error=e.message;}finally{this.decodeBusy=false;}},
  async save(){
   if(this.decodeBusy||this.busy)return;const f=this.form;
   const entries=f.kind==='subscription'?[...new Set(f.content.split(/\r?\n/).map(x=>x.trim()).filter(Boolean))]:[f.content.trim()];
   if(!entries.length||!entries[0]){this.error='请填写订阅地址、节点链接或识别二维码';return;}
   if(f.kind==='subscription'&&entries.some(x=>{try{return new URL(x).protocol!=='https:';}catch{return true;}})){this.error='每行填写一个完整的 HTTPS 订阅链接';return;}
   if(this.state.sources.length+entries.length>20){this.error='最多添加 20 个来源，请减少本次导入数量';return;}
   this.busy=true;this.error='';this.notice='';let done=0;
   try{for(const entry of entries){if(!this._alive)return;const state=await this.request('import',{version:this.state.version,name:entries.length>1?`${f.name.trim()||'新订阅'} ${done+1}`:f.name,kind:f.kind,content:entry});if(!this._alive)return;this.state=state;done++;f.content=entries.slice(done).join('\n');}
    this.notice='';
   }catch(e){if(this._alive){this.error=(done?`已导入 ${done} 个来源，其余链接已保留。`:'')+e.message;await this.refresh();}}
   finally{this.busy=false;}
   if(this._alive&&!this.error)this.close();
  },
  async mutate(action,payload={}){this.busy=true;this.error='';this.notice='';try{const s=await this.request(action,{version:this.state.version,...payload});if(!this._alive)return;if(s.nodes)this.state=s;this.publishStatus();if(action==='refresh'){for(const n of this.state.nodes.filter(n=>n.source_id===payload.source_id))delete this.results[n.id];}this.confirmAction=null;if(action==='select'||action==='pause')this.$emit('updated');if(action==='select')await this.measureLatency();}catch(e){if(this._alive){this.error=e.message;await this.refresh();this.$emit('updated');}}finally{this.busy=false;}},
  confirm(action,node){this.focusReturn=document.activeElement;this.confirmAction={action,node};this.$nextTick(()=>this.$refs.cancel?.focus());},
  async test(node){this.testId=node.id;try{const r=await this.request('test',{node_id:node.id});if(this._alive)this.results[node.id]={ok:true,text:`HTTPS ${r.delay_ms} ms`};return true;}catch(e){if(this._alive)this.results[node.id]={ok:false,text:'连接失败或超时'};return false;}finally{this.testId='';}},
  async testSource(source){
   if(this.batch||this.testId||this.busy)return;
   const nodes=this.state.nodes.filter(n=>n.source_id===source.id);
   this.batch={sourceId:source.id,cancelled:false};const batch=this.batch;
   for(const n of nodes)delete this.results[n.id];
   for(const node of nodes){if(!this._alive||batch.cancelled)break;await this.test(node);}
   this.batch=null;
  },
  nodeList(nodes,label){
   if(!nodes.length)return h('div',{class:'mp-empty'},'没有匹配的节点');
   return h('div',{class:'mp-node-table'},[
    h('div',{class:'mp-node-columns','aria-hidden':true},['节点名称','协议','延迟','操作'].map(t=>h('span',null,t))),
    h('div',{class:'mp-nodes',role:'region','aria-label':label+'节点列表，可上下滚动',tabindex:0},nodes.map(n=>{
     const active=this.state.enabled&&this.state.selected===n.id,result=this.results[n.id];
     return h('article',{class:'mp-node'+(active?' selected':''),key:n.id},[
      h('div',{class:'mp-node-name'},[h('strong',null,n.name),active?h('small',{class:'mp-selected-label'},'使用中'):null,!n.udp?h('small',null,'UDP 未启用'):null]),
      h('span',{class:'mp-type',title:n.udp?'UDP 已配置':'UDP 未启用'},n.type.toUpperCase()),
      h('span',{class:'mp-delay '+(result?(result.ok?'mp-ok':'mp-failed'):'mp-unmeasured')},result?result.text.replace(/^HTTPS /,''):'—'),
      h('div',{class:'mp-actions'},[this.button('测试',()=>this.test(n),false,{'aria-busy':this.testId===n.id}),this.button(active?'使用中':'使用',()=>this.confirm('select',n),false,{disabled:this.busy||!!this.batch||!!this.testId||!n.udp||active})])
     ]);
    }))
   ]);
  },
  dialog(title,children,footer){return h('div',{class:'mp-overlay',onClick:e=>{if(e.target===e.currentTarget)this.close();}},[h('section',{class:'mp-dialog',role:'dialog','aria-modal':'true','aria-label':title,onKeydown:e=>{if(e.key==='Escape')this.close();if(e.key==='Tab'){const els=[...e.currentTarget.querySelectorAll('button:not(:disabled),input:not(:disabled),textarea:not(:disabled),select:not(:disabled)')];const a=els[0],b=els.at(-1);if(e.shiftKey&&document.activeElement===a){e.preventDefault();b?.focus();}else if(!e.shiftKey&&document.activeElement===b){e.preventDefault();a?.focus();}}}},[h('header',{class:'mp-dialog-header'},[h('h3',null,title),h('button',{type:'button',class:'schedule-close mp-dialog-close','aria-label':'关闭',disabled:this.busy||this.decodeBusy,onClick:()=>this.close()})]),h('div',{class:'mp-dialog-body'},children),h('footer',null,footer)])]);}
 },
 render(){
  const s=this.state;const selected=s.nodes.find(n=>n.id===s.selected);const nodes=s.nodes;
  const content=[h('link',{rel:'stylesheet',href:'/assets/ManagedProxy-DJI26006.css'}),h('header',{class:'mp-heading'},[h('div',null,[h('h2',null,'订阅与节点')]),h('div',{class:'mp-heading-actions'},[this.button('添加订阅 / 节点',()=>this.open(),true)])]),h('div',{class:'mp-current'},[h('div',null,[h('span',{class:'mp-label'},'当前出口'),h('strong',null,selected?.name||'未选择节点'),h('span',{class:'mp-connection-state'},s.enabled?'已连接':'未连接'),s.error?h('small',null,s.error):null]),h('div',{class:'mp-actions'},[this.button(s.enabled?'断开':'连接',()=>s.enabled?this.mutate('pause'):this.connect(),!s.enabled,{class:'mp-button '+(s.enabled?'mp-disconnect':'primary'),disabled:this.busy||this.loading||!!this.batch||!!this.testId||(!s.enabled&&!selected)})])]),
  componentVNode(EgressIPs,{connectionKey:s.version+':'+s.enabled+':'+s.selected}),
  this.error?h('div',{class:'mp-message error',role:'alert'},this.error):null,this.notice?h('div',{class:'mp-message',role:'status'},this.notice):null,
  s.sources.length?h('div',{class:'mp-sources'},s.sources.map(x=>{
   const collapsed=this.sourceCollapsed[x.id]!==false,batch=this.batch?.sourceId===x.id?this.batch:null;
   const filtered=nodes.filter(n=>n.source_id===x.id),detailsId='mp-source-'+x.id;
   return h('section',{class:'mp-source-group',key:x.id},[
    h('header',{class:'mp-source'},[h('div',{class:'mp-source-info'},[h('strong',null,x.name),h('small',null,`${x.count??filtered.length} 个节点 · 更新时间 `+format(x.updated))]),h('div',{class:'mp-actions'},[
     x.kind==='subscription'?this.button('更新',()=>this.mutate('refresh',{source_id:x.id})):null,
     batch?this.button('停止测试',()=>batch.cancelled=true,false,{disabled:batch.cancelled}):this.button('一键测试',()=>this.testSource(x),false,{'aria-label':'一键测试 '+x.name}),
     this.button('删除',()=>this.confirm('delete',x)),
     this.button((collapsed?'展开':'收起')+` (${x.count})`,()=>this.toggleSource(x.id),false,{'aria-label':(collapsed?'展开 ':'收起 ')+x.name,'aria-expanded':!collapsed,'aria-controls':detailsId,disabled:false})
    ])]),
    h('div',{id:detailsId,hidden:collapsed,class:'mp-source-body'},[this.nodeList(filtered,x.name+' ')])
   ]);
  })):h('div',{class:'mp-empty'},this.loading?'正在加载…':'还没有导入节点。可一次添加多个订阅链接，每行一个。'),
  ];
  const details=content.splice(2);content.push(h('div',{id:'managed-proxy-details',class:'mp-body'},details));
  if(this.form){const f=this.form;content.push(this.dialog('添加订阅 / 节点',[h('label',{class:'mp-field'},[h('span',null,'名称'),h('input',{ref:'name',value:f.name,maxlength:100,onInput:e=>f.name=e.target.value})]),h('label',{class:'mp-field'},[h('span',null,'导入方式'),h('select',{value:f.kind,onChange:e=>{f.kind=e.target.value;f.content='';}},[h('option',{value:'subscription'},'订阅链接'),h('option',{value:'nodes'},'节点链接 / Clash YAML')])]),h('label',{class:'mp-field'},[h('span',null,f.kind==='subscription'?'HTTPS 订阅地址（每行一个，可添加多个）':'节点内容'),h('textarea',{value:f.content,rows:f.kind==='subscription'?5:6,autocomplete:'off',autocorrect:'off',autocapitalize:'off',spellcheck:false,placeholder:f.kind==='subscription'?'https://订阅地址一\nhttps://订阅地址二':'vmess://…、vless://… 或 Clash YAML',onInput:e=>f.content=e.target.value})]),h('div',{class:'mp-qr'},[h('input',{ref:'image',type:'file',accept:'image/png,image/jpeg,image/webp',hidden:true,onChange:e=>this.decode(e)}),this.button(this.decodeBusy?'识别中…':'上传二维码图片',()=>this.$refs.image.click(),false,{disabled:this.busy||this.decodeBusy}),h('small',null,'图片仅在浏览器内识别，完成或失败后立即清空，不上传、不缓存。')]),this.error?h('div',{class:'mp-message error',role:'alert'},this.error):null],[this.button('取消',()=>this.close(),false,{disabled:this.busy||this.decodeBusy}),this.button(this.busy?'导入中…':'确认导入',()=>this.save(),true,{disabled:this.busy||this.decodeBusy})]));}
  if(this.confirmAction){const {action,node}=this.confirmAction;const title=action==='select'?'切换代理出口':action==='pause'?'断开代理出口':'删除来源';content.push(this.dialog(title,[h('p',null,action==='select'?`使用“${node.name}”作为出口？已关联的 VoWiFi 连接可能需要重新注册。`:action==='pause'?'断开后，内置代理将关闭。':`删除“${node.name}”及其导入的节点？`),this.error?h('div',{class:'mp-message error',role:'alert'},this.error):null],[this.button('取消',()=>this.close(),false,{ref:'cancel'}),this.button('确认',()=>this.mutate(action,action==='select'?{node_id:node.id}:action==='delete'?{source_id:node.id}:{}),true)]));}
  return h('section',{class:'mp-panel'},content);
 }
};
