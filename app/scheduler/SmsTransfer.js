import { X as h } from '/assets/vendor-DJI26003.js';
import { parseArchive, serializeMessages, groupConversations, conversationKey } from './sms-transfer-codec-DJI26003.js';

const FORMATS=['csv','txt','html','xml'];
const MIME={csv:'text/csv;charset=utf-8',txt:'text/plain;charset=utf-8',html:'text/html;charset=utf-8',xml:'application/xml;charset=utf-8'};
const EXTENSIONS={htm:'html'};
const CLOSE_PATH='M11.9997 10.5865L16.9495 5.63672L18.3637 7.05093L13.4139 12.0007L18.3637 16.9504L16.9495 18.3646L11.9997 13.4149L7.04996 18.3646L5.63574 16.9504L10.5855 12.0007L5.63574 7.05093L7.04996 5.63672L11.9997 10.5865Z';
const CONTEXTS=new WeakMap;

function authorization(){return {'Authorization':'Bearer '+(localStorage.getItem('token')||'')};}
async function request(path,options={}){
 const response=await fetch(path,{cache:'no-store',...options,headers:{...authorization(),...(options.headers||{})}});
 let value;try{value=await response.json();}catch{throw Error('服务返回异常，请稍后重试');}
 if(!response.ok)throw Error(value.error||'操作失败');return value;
}
function phoneOf(device){const modem=device?.modem||{};return String(device?.local_phone||device?.phone_number||modem.msisdn||modem.phone_number||'').trim();}
function titleOf(device){return device?.name||device?.id||'未知设备';}
function safeFilename(value){return String(value||'device').replace(/[\\/:*?"<>|\s]+/g,'-').replace(/^-+|-+$/g,'').slice(0,60)||'device';}
function node(tag,className,text){const element=document.createElement(tag);if(className)element.className=className;if(text!==undefined)element.textContent=text;return element;}
function option(value,label){const element=node('option','',label);element.value=value;return element;}
function checkbox(checked){const element=node('input');element.type='checkbox';element.checked=checked;return element;}
function context(instance){let value=CONTEXTS.get(instance);if(!value){value={state:null,overlay:null,returnFocus:null,nativeClick:null,escapeHandler:null};CONTEXTS.set(instance,value);}return value;}

export default {
 name:'SmsTransfer',
 props:{devices:{type:Array,default:()=>[]},currentDevice:{type:String,default:'all'}},
 emits:['changed'],
 mounted(){
  const value=context(this),report=error=>console.error('SmsTransfer open failed',error);value.nativeClick=event=>{const button=event.target.closest?.('[data-sms-action]');if(!button||!this.$el.contains(button))return;const action=button.dataset.smsAction;if(action==='open-delete')this.openDialog('delete').catch(report);else if(action==='open-import')this.openDialog('import').catch(report);else if(action==='open-export')this.openDialog('export').catch(report);};
  value.escapeHandler=event=>{if(event.key==='Escape')this.close();};
  this.$el.addEventListener('click',value.nativeClick);document.addEventListener('keydown',value.escapeHandler);
 },
 beforeUnmount(){const value=context(this);this.$el?.removeEventListener('click',value.nativeClick);document.removeEventListener('keydown',value.escapeHandler);this.removeOverlay();CONTEXTS.delete(this);},
 methods:{
  defaultDevice(){const current=this.currentDevice!=='all'&&this.devices.some(device=>device.id===this.currentDevice)?this.currentDevice:'';return current||this.devices[0]?.id||'';},
  getDevice(){return this.devices.find(device=>device.id===context(this).state?.deviceId)||null;},
  removeOverlay(){const value=context(this);value.overlay?.remove();value.overlay=null;},
  close(){const value=context(this);if(value.state?.busy)return;this.removeOverlay();value.state=null;this.$nextTick(()=>value.returnFocus?.focus());},
  async openDialog(mode){
   const value=context(this);value.returnFocus=document.activeElement;value.state={mode,deviceId:this.defaultDevice(),format:'csv',contacts:[],selected:[],loading:false,busy:false,error:'',notice:'',fileName:'',importRows:[],importGroups:[],deleteConfirm:false};
   this.renderDialog();if(mode!=='import')await this.loadContacts(mode==='export');
  },
  items(){const state=context(this).state;return state.mode==='import'?state.importGroups:state.contacts;},
  async loadContacts(selectAll){
   const state=context(this).state;if(!state?.deviceId){state.contacts=[];state.selected=[];this.renderDialog();return;}
   state.loading=true;state.error='';this.renderDialog();
   try{const params=new URLSearchParams({device_id:state.deviceId,limit:'500'});const rows=await request('/api/sms/contacts?'+params);if(context(this).state!==state)return;state.contacts=rows.map(row=>({...row,key:`${row.imsi||''}|${row.peer||''}`}));state.selected=selectAll?state.contacts.map(row=>row.key):[];}
   catch(error){if(context(this).state===state)state.error=error.message;}
   finally{if(context(this).state===state){state.loading=false;this.renderDialog();}}
  },
  async fetchThread(contact){
   const state=context(this).state,found=new Map;let beforeTs='',beforeId='',previous='';
   for(let page=0;page<100;page++){
    const params=new URLSearchParams({device_id:state.deviceId,peer:contact.peer,imsi:contact.imsi||'',limit:'200'});if(beforeTs){params.set('before_ts',beforeTs);params.set('before_id',beforeId);}
    const rows=await request('/api/sms/thread?'+params);for(const row of rows){const key=`${row.id}|${row.timestamp}|${row.type}|${row.content}`;found.set(key,{...row,device_id:state.deviceId,device_name:row.device_name||titleOf(this.getDevice()),local_phone:row.local_phone||phoneOf(this.getDevice()),imsi:row.imsi||contact.imsi||'',peer:row.peer||contact.peer,message_id:row.id});}
    if(rows.length<200)break;const first=rows[0],cursor=`${first.timestamp}|${first.id}`;if(cursor===previous)break;previous=cursor;beforeTs=first.timestamp;beforeId=String(first.id||0);
   }
   return [...found.values()].sort((a,b)=>Date.parse(a.timestamp)-Date.parse(b.timestamp)||(Number(a.id)||0)-(Number(b.id)||0));
  },
  toggle(key,checked){const state=context(this).state,next=new Set(state.selected);checked?next.add(key):next.delete(key);state.selected=[...next];state.deleteConfirm=false;this.renderDialog();},
  toggleAll(checked){const state=context(this).state;state.selected=checked?this.items().map(item=>item.key):[];state.deleteConfirm=false;this.renderDialog();},
  async fileChanged(file){
   const state=context(this).state;if(!file)return;state.error='';state.notice='';state.fileName=file.name;
   const extension=(file.name.split('.').pop()||'').toLowerCase(),format=EXTENSIONS[extension]||extension;if(!FORMATS.includes(format)){state.error='仅支持 CSV、TXT、HTML、XML';this.renderDialog();return;}if(file.size>16*1024*1024){state.error='文件不能超过 16 MB';this.renderDialog();return;}
   state.loading=true;this.renderDialog();try{const rows=parseArchive(await file.text(),format);if(context(this).state!==state)return;state.importRows=rows;state.importGroups=groupConversations(rows);state.selected=state.importGroups.map(group=>group.key);state.notice=`已读取 ${state.importGroups.length} 个会话，共 ${rows.length} 条短信`;}
   catch(error){state.importRows=[];state.importGroups=[];state.selected=[];state.error=error.message;}
   finally{if(context(this).state===state){state.loading=false;this.renderDialog();}}
  },
  async exportSelected(){
   const state=context(this).state,chosen=state.contacts.filter(contact=>state.selected.includes(contact.key));if(!chosen.length){state.error='请至少选择一个会话';this.renderDialog();return;}
   state.busy=true;state.error='';state.notice='';this.renderDialog();try{const rows=[];for(let index=0;index<chosen.length;index++){state.notice=`正在读取 ${index+1} / ${chosen.length} 个会话…`;this.renderDialog();rows.push(...await this.fetchThread(chosen[index]));}const body=serializeMessages(rows,state.format),blob=new Blob([body],{type:MIME[state.format]});const url=URL.createObjectURL(blob),link=document.createElement('a');link.href=url;link.download=`VoHiveX-SMS-${safeFilename(titleOf(this.getDevice()))}-${new Date().toISOString().slice(0,10)}.${state.format}`;document.body.appendChild(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(url),1000);state.notice=`已导出 ${chosen.length} 个会话，共 ${rows.length} 条短信`;}
   catch(error){state.error=error.message;}finally{state.busy=false;this.renderDialog();}
  },
  async importSelected(){
   const state=context(this).state;if(!state.deviceId){state.error='请选择导入设备';this.renderDialog();return;}const selected=new Set(state.selected),rows=state.importRows.filter(row=>selected.has(conversationKey(row)));if(!rows.length){state.error='请至少选择一个会话';this.renderDialog();return;}
   state.busy=true;state.error='';this.renderDialog();try{const value=await request('/api/sms/archive/import',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({device_id:state.deviceId,messages:rows})});state.notice=`已导入 ${value.inserted} 条短信${value.skipped?`，跳过 ${value.skipped} 条重复记录`:''}`;this.$emit('changed');}
   catch(error){state.error=error.message;}finally{state.busy=false;this.renderDialog();}
  },
  async deleteSelected(){
   const state=context(this).state,chosen=state.contacts.filter(contact=>state.selected.includes(contact.key));if(!chosen.length){state.error='请至少选择一个会话';this.renderDialog();return;}if(!state.deleteConfirm){state.deleteConfirm=true;state.error='';this.renderDialog();return;}
   state.busy=true;state.error='';this.renderDialog();let deleted=0;const failures=[];for(const contact of chosen){try{const params=new URLSearchParams({device_id:state.deviceId,imsi:contact.imsi||'',peer:contact.peer});await request('/api/sms/thread?'+params,{method:'DELETE'});deleted++;}catch(error){failures.push(`${contact.peer}：${error.message}`);}}
   state.busy=false;state.deleteConfirm=false;state.notice=`已删除 ${deleted} 个会话`;if(failures.length)state.error=`${failures.length} 个会话删除失败：${failures.slice(0,3).join('；')}`;this.$emit('changed');await this.loadContacts(false);
  },
  createCloseButton(){const button=node('button','sms-transfer-close');button.type='button';button.setAttribute('aria-label','关闭');button.disabled=context(this).state.busy;const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('viewBox','0 0 24 24');svg.setAttribute('fill','currentColor');svg.setAttribute('aria-hidden','true');const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',CLOSE_PATH);svg.appendChild(path);button.appendChild(svg);button.addEventListener('click',()=>this.close());return button;},
  renderDialog(){
   const value=context(this),state=value.state;if(!state)return;this.removeOverlay();const names={import:'导入短信会话',export:'导出短信会话',delete:'批量删除会话'},overlay=node('div','sms-transfer-overlay'),dialog=node('section','sms-transfer-dialog');dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label',names[state.mode]);overlay.addEventListener('click',event=>{if(event.target===overlay)this.close();});
   const header=node('header');header.append(node('h3','',names[state.mode]),this.createCloseButton());dialog.appendChild(header);const body=node('div','sms-transfer-body');
   const deviceLabel=node('label','sms-transfer-field'),deviceSelect=node('select');deviceSelect.disabled=state.busy;for(const device of this.devices)deviceSelect.appendChild(option(device.id,titleOf(device)));deviceSelect.value=state.deviceId;deviceSelect.addEventListener('change',event=>{state.deviceId=event.target.value;state.deleteConfirm=false;state.error='';state.notice='';if(state.mode==='import')this.renderDialog();else void this.loadContacts(state.mode==='export');});deviceLabel.append(node('span','','设备'),deviceSelect,node('small','sms-transfer-phone',`设备手机号码：${phoneOf(this.getDevice())||'未读取到设备手机号码'}`));body.appendChild(deviceLabel);
   if(state.mode==='import'){const fileLabel=node('label','sms-transfer-file'),fileInput=node('input');fileInput.type='file';fileInput.accept='.csv,.txt,.html,.htm,.xml,text/csv,text/plain,text/html,application/xml,text/xml';fileInput.disabled=state.busy;fileInput.addEventListener('change',event=>void this.fileChanged(event.target.files?.[0]));fileLabel.append(node('span','','选择短信归档文件'),fileInput,node('small','',state.fileName||'支持 CSV、TXT、HTML、XML，最多 16 MB'));body.appendChild(fileLabel);}
   if(state.mode==='export'){const formatLabel=node('label','sms-transfer-field'),formatSelect=node('select');for(const format of FORMATS)formatSelect.appendChild(option(format,format.toUpperCase()));formatSelect.value=state.format;formatSelect.disabled=state.busy;formatSelect.addEventListener('change',event=>{state.format=event.target.value;});formatLabel.append(node('span','','导出格式'),formatSelect);body.appendChild(formatLabel);}
   const items=this.items();if(items.length){const allLabel=node('label','sms-transfer-select-all'),allInput=checkbox(state.selected.length===items.length);allInput.disabled=state.busy;allInput.addEventListener('change',event=>this.toggleAll(event.target.checked));allLabel.append(allInput,node('span','',`全选会话（${items.length}）`));body.appendChild(allLabel);}
   if(state.loading)body.appendChild(node('p','sms-transfer-muted','正在读取…'));else if(!items.length)body.appendChild(node('p','sms-transfer-empty',state.mode==='import'?'选择文件后会在这里显示会话':'当前设备没有可选择的会话'));else{const list=node('div','sms-transfer-conversations');for(const item of items){const label=node('label','sms-transfer-conversation'),input=checkbox(state.selected.includes(item.key)),content=node('span','sms-transfer-contact'),strong=node('strong','',item.peer),small=node('small','',state.mode==='import'?`${item.count} 条 · ${new Date(item.lastTimestamp).toLocaleString()}`:`${item.local_phone||item.lastDeviceName||titleOf(this.getDevice())} · ${item.lastMessage||''}`);strong.title=item.peer;input.disabled=state.busy;input.addEventListener('change',event=>this.toggle(item.key,event.target.checked));content.append(strong,small);label.append(input,content);list.appendChild(label);}body.appendChild(list);}
   if(state.mode==='delete'&&state.deleteConfirm)body.appendChild(node('div','sms-transfer-warning',`将永久删除选中的 ${state.selected.length} 个会话，无法恢复。仅删除短信中心历史记录。`));if(state.notice)body.appendChild(node('p','sms-transfer-notice',state.notice));if(state.error)body.appendChild(node('p','sms-transfer-error',state.error));dialog.appendChild(body);
   const footer=node('footer'),cancel=node('button','sms-transfer-button','取消'),submit=node('button',`sms-transfer-button ${state.mode==='delete'&&state.deleteConfirm?'danger':'primary'}`,state.busy?'处理中…':state.mode==='import'?'导入所选会话':state.mode==='export'?'导出所选会话':state.deleteConfirm?'永久删除所选会话':'继续删除');cancel.type=submit.type='button';cancel.disabled=state.busy;submit.disabled=state.busy||!state.selected.length;cancel.addEventListener('click',()=>this.close());submit.addEventListener('click',()=>{if(state.mode==='import')void this.importSelected();else if(state.mode==='export')void this.exportSelected();else void this.deleteSelected();});footer.append(cancel,submit);dialog.appendChild(footer);overlay.appendChild(dialog);document.body.appendChild(overlay);value.overlay=overlay;
   queueMicrotask(()=>{if(context(this).overlay===overlay)deviceSelect.focus();});
  }
 },
 render(){return h('div',{class:'sms-transfer'},[
  h('button',{type:'button',class:'sms-transfer-action','data-sms-action':'open-import'},'导入'),
  h('button',{type:'button',class:'sms-transfer-action','data-sms-action':'open-export'},'导出')
 ]);}
};
