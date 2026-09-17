import { X as h, V as v, c as ref } from '/assets/vendor-DJI26003.js';
import { Remix } from './RemixIcons-DJI26003.js';

export const selectionActive=ref(false);
const selectedKeys=ref([]);

function selected(key){return selectedKeys.value.includes(key);}
function clearSelection(){selectedKeys.value=[];}
function toggleItem(item){const next=new Set(selectedKeys.value);next.has(item.key)?next.delete(item.key):next.add(item.key);selectedKeys.value=[...next];}
export function isSelectedThread(key){return selected(key);}

function authorization(){return {'Authorization':'Bearer '+(localStorage.getItem('token')||'')};}
async function deleteThread(item,currentDevice){
 const params=new URLSearchParams({device_id:currentDevice!=='all'?currentDevice:'all',peer:item.peer});
 if(currentDevice==='all'&&item.imsi)params.set('imsi',item.imsi);
 const response=await fetch('/api/sms/thread?'+params,{method:'DELETE',cache:'no-store',headers:authorization()});
 let value={};try{value=await response.json();}catch{}
 if(!response.ok)throw Error(value?.error||'删除失败');
 return value;
}

export const SmsSelectionToggle={
 name:'SmsSelectionToggle',
 setup(){return()=>h('button',{type:'button',class:selectionActive.value?'sms-select-toggle is-cancel':'sms-select-toggle',onClick:()=>{selectionActive.value=!selectionActive.value;clearSelection();}},selectionActive.value?'取消选择':'选择');}
};

export const SmsSelectionCheck={
 name:'SmsSelectionCheck',
 props:{item:{type:Object,required:true}},
 setup(props){return()=>selectionActive.value?h('button',{type:'button',class:selected(props.item.key)?'sms-thread-select is-selected':'sms-thread-select','aria-label':selected(props.item.key)?`取消选择 ${props.item.peer}`:`选择 ${props.item.peer}`,onClick:event=>{event.preventDefault();event.stopPropagation();toggleItem(props.item);}},selected(props.item.key)?[v(Remix.check)]:[]):null;}
};

export const SmsSelectionActions={
 name:'SmsSelectionActions',
 props:{items:{type:Array,default:()=>[]},currentDevice:{type:String,default:'all'}},
 emits:['changed'],
 setup(props,{emit}){
  const pending=ref(''),busy=ref(false),error=ref('');
  const chosen=()=>props.items.filter(item=>selected(item.key));
  const labels={read:'标记已读',unread:'标记未读',delete:'删除'};
  function ask(action){if(!chosen().length)return;pending.value=action;error.value='';}
  function cancel(){if(!busy.value){pending.value='';error.value='';}}
  async function execute(){
   const action=pending.value,items=chosen();if(!action||!items.length)return;
   busy.value=true;error.value='';
   try{
    if(action==='read'||action==='unread'){
     for(const item of items){const key=`sms_thread_last_seen:${props.currentDevice}:${item.key}`;try{action==='read'?localStorage.setItem(key,String(item.lastTs||Date.now())):localStorage.removeItem(key);}catch{}}
    }else{
     const failures=[];for(const item of items)try{await deleteThread(item,props.currentDevice);}catch(reason){failures.push(`${item.peer}：${reason.message||'删除失败'}`);}
     if(failures.length)throw Error(`${failures.length} 个会话删除失败`);
    }
    clearSelection();selectionActive.value=false;pending.value='';emit('changed');
   }catch(reason){error.value=reason.message||'操作失败，请稍后重试';}
   finally{busy.value=false;}
  }
  return()=>{
   if(!selectionActive.value)return null;
   const count=chosen().length,action=pending.value,label=labels[action]||'',description=action==='delete'?`将永久删除选中的 ${count} 个会话，无法恢复。`:`将把选中的 ${count} 个会话${label}。`;
   return h('div',{class:'sms-selection-actions'},[
    h('button',{type:'button',disabled:count===0,onClick:()=>ask('read')},'标记已读'),
    h('button',{type:'button',disabled:count===0,onClick:()=>ask('unread')},'标记未读'),
    h('button',{type:'button',class:'danger',disabled:count===0,onClick:()=>ask('delete')},'删除'),
    action?h('div',{class:'sms-selection-confirm-overlay',onClick:event=>{if(event.target===event.currentTarget)cancel();}},[
     h('section',{class:'sms-selection-confirm',role:'dialog','aria-modal':'true','aria-label':`${label}所选会话`},[
      h('header',null,[h('h3',null,`${label}所选会话？`),h('button',{type:'button',class:'sms-selection-confirm-close','aria-label':'关闭',disabled:busy.value,onClick:cancel},[v(Remix.close)])]),
      h('p',null,description),
      error.value?h('p',{class:'sms-selection-confirm-error',role:'alert'},error.value):null,
      h('footer',null,[h('button',{type:'button',disabled:busy.value,onClick:cancel},'取消'),h('button',{type:'button',class:action==='delete'?'danger':'primary',disabled:busy.value,onClick:execute},busy.value?'处理中…':label)])
     ])
    ]):null
   ]);
  };
 }
};
