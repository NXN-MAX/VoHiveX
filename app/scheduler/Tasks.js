import { X as h } from '/assets/vendor-DJI26005.js';
const TZ = 'Asia/Shanghai';
const dateFormat = new Intl.DateTimeFormat('zh-CN', {timeZone:TZ,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false});
function formatTime(t) { return t ? dateFormat.format(new Date(t*1000)) : '—'; }
function inputTime(t) { const p = Object.fromEntries(dateFormat.formatToParts(new Date(t*1000)).map(x=>[x.type,x.value]));return `${p.year}-${p.month}-${p.day}T${p.hour==='24'?'00':p.hour}:${p.minute}:${p.second}`; }
function intervalText(t) { let n=t.interval_seconds;const out=[];for(const [s,u] of [[86400,'天'],[3600,'小时'],[60,'分钟'],[1,'秒']]){const v=Math.floor(n/s);if(v)out.push(`${v} ${u}`);n%=s;}return '每隔 '+out.join(' '); }
const states={active:'进行中',paused:'已暂停',completed:'已完成'};
async function request(path='', method='GET', body) {
 const response=await fetch('/api/schedules'+path,{method,headers:{Authorization:'Bearer '+(localStorage.getItem('token')||''),'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body),cache:'no-store'});
 let value;try{value=await response.json();}catch{throw Error('服务返回异常，请稍后重试');}
 if(!response.ok)throw Error(value.error||'操作失败');return value;
}
export default {
 name:'ScheduledTasks',
 data(){return {tasks:[],devices:[],loading:true,error:'',notice:'',editing:null,form:null,formError:'',busy:false,actionId:'',deleteTask:null,historyTask:null,runs:[],historyError:'',timer:null,returnFocus:null};},
 mounted(){this.refresh();this.timer=setInterval(()=>{if(!document.hidden)this.refresh(false);},3000);this.keyHandler=e=>{if(e.key==='Escape'&&!this.busy)this.closeDialog();};document.addEventListener('keydown',this.keyHandler);},
 beforeUnmount(){clearInterval(this.timer);document.removeEventListener('keydown',this.keyHandler);},
 methods:{
  async refresh(initial=true){try{const [a,b]=await Promise.all([request(),request('/devices')]);this.tasks=a.tasks;this.devices=b.devices;this.error='';}catch(e){this.error=e.message;}finally{this.loading=false;}},
  deviceName(id){return this.devices.find(d=>d.id===id)?.name||id;},
  open(task){this.returnFocus=document.activeElement;this.editing=task?{...task}:null;this.formError='';let n=task?.interval_seconds||86400;const days=Math.floor(n/86400);n%=86400;const hours=Math.floor(n/3600);n%=3600;const minutes=Math.floor(n/60);const seconds=n%60;this.form={name:task?.name||'定时短信',device_id:task?.device_id||this.devices[0]?.id||'',phone:task?.phone||'',message:task?.message||'',mode:task?.mode||'once',first:inputTime(Math.max(task?.first_run||0,Math.floor(Date.now()/1000)+300)),days,hours,minutes,seconds};this.$nextTick(()=>this.$refs.taskName?.focus());},
  closeDialog(){if(this.busy)return;this.form=null;this.deleteTask=null;this.historyTask=null;this.$nextTick(()=>this.returnFocus?.focus());},
  async save(){this.formError='';const f=this.form;const values=['days','hours','minutes','seconds'].map(k=>Number(f[k]));if(f.mode==='interval'&&values.some(x=>!Number.isInteger(x)||x<0)){this.formError='间隔请填写非负整数';return;}const first=Date.parse(f.first+'+08:00')/1000;const payload={name:f.name,device_id:f.device_id,phone:f.phone,message:f.message,mode:f.mode,first_run:first,interval_seconds:f.mode==='interval'?values[0]*86400+values[1]*3600+values[2]*60+values[3]:0};if(!Number.isFinite(first)){this.formError='请填写执行时间';return;}this.busy=true;try{if(this.editing){await request('/'+this.editing.id,'PUT',{...payload,version:this.editing.version});}else await request('','POST',payload);this.notice=this.editing?'任务已修改并暂停，点击“开始”后执行':'任务已创建并暂停，点击“开始”后执行';this.form=null;await this.refresh(false);this.$nextTick(()=>this.returnFocus?.focus());}catch(e){this.formError=e.message;}finally{this.busy=false;}},
  async action(task,action){this.actionId=task.id;this.error='';try{await request('/'+task.id+'/'+action,'POST',{version:task.version});this.notice=action==='start'?'任务已开始，将按下次执行时间发送':'任务已暂停；正在发送的本次短信无法撤回';await this.refresh(false);}catch(e){this.error=e.message;}finally{this.actionId='';}},
  confirmDelete(task){this.returnFocus=document.activeElement;this.deleteTask=task;this.formError='';this.$nextTick(()=>this.$refs.cancelDelete?.focus());},
  async remove(){this.busy=true;try{await request('/'+this.deleteTask.id,'DELETE',{version:this.deleteTask.version});this.deleteTask=null;this.notice='任务已删除';await this.refresh(false);}catch(e){this.formError=e.message;}finally{this.busy=false;}},
  async showHistory(task){this.returnFocus=document.activeElement;this.historyTask=task;this.runs=[];this.historyError='';try{this.runs=(await request('/'+task.id+'/history')).runs;}catch(e){this.historyError=e.message;}},
  field(label,node,help){return h('label',{class:'schedule-field'},[h('span',null,label),node,help?h('small',null,help):null]);},
  input(key,props={}){return h('input',{value:this.form[key],onInput:e=>this.form[key]=e.target.value,...props});},
  button(text,click,cls='',props={}){return h('button',{type:'button',class:'schedule-button '+cls,onClick:click,...props},text);},
  dialog(title,body,footer){return h('div',{class:'schedule-overlay',onClick:e=>{if(e.target===e.currentTarget)this.closeDialog();}},[h('section',{class:'schedule-dialog',role:'dialog','aria-modal':'true','aria-label':title,onKeydown:e=>{if(e.key==='Tab'){const nodes=[...e.currentTarget.querySelectorAll('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled)')];const first=nodes[0],last=nodes[nodes.length-1];if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus();}}}},[h('header',null,[h('h3',null,title),this.button('关闭',()=>this.closeDialog(),'schedule-close',{'aria-label':'关闭',disabled:this.busy})]),h('div',{class:'schedule-dialog-body'},body),h('footer',null,footer)])]);}
 },
 render(){
  const active=this.tasks.filter(t=>t.state==='active');const next=active.filter(t=>t.next_run).sort((a,b)=>a.next_run-b.next_run)[0];
  const blocks=[h('link',{rel:'stylesheet',href:'/assets/ScheduledTasks-DJI26005.css'}),h('div',{class:'schedule-heading vh-page-heading'},[h('div',null,[h('h2',{class:'vh-page-title'},'定时任务'),h('p',{class:'vh-page-subtitle'},'按指定时间或固定间隔发送短信')]),this.button('新增任务',()=>this.open(null),'primary',{disabled:this.loading||!this.devices.length})]),
   h('div',{class:'schedule-summary'},[[String(active.length),'进行中的任务'],[String(this.tasks.filter(t=>t.state==='paused').length),'已暂停的任务'],[next?formatTime(next.next_run):'暂无安排','最近执行时间 · 北京时间']].map(([v,l])=>h('div',{class:'schedule-stat'},[h('span',null,l),h('strong',null,v)]))),
   this.error?h('div',{class:'schedule-alert error',role:'alert'},[this.error,this.button('重试',()=>this.refresh())]):null,
   this.notice?h('div',{class:'schedule-alert',role:'status'},this.notice):null,
   h('div',{class:'schedule-list-head'},[h('h3',null,`全部任务 (${this.tasks.length})`),h('span',null,'北京时间（UTC+8）')])];
  if(this.loading)blocks.push(h('div',{class:'schedule-empty'},'正在加载任务…'));
  else if(!this.tasks.length)blocks.push(h('div',{class:'schedule-empty'},[h('div',{class:'schedule-empty-icon'},'◷'),h('h3',null,'还没有定时任务'),h('p',null,this.devices.length?'创建任务，安排下一条短信。保存后点击“开始”即可启用。':'请先在设备管理中添加发信设备。'),this.button('新增任务',()=>this.open(null),'primary',{disabled:!this.devices.length})]));
  for(const task of this.tasks){const sending=task.last_result==='发送中';blocks.push(h('article',{class:'schedule-task',key:task.id},[
   h('div',{class:'schedule-task-title'},[h('h3',null,task.name),h('span',{class:'schedule-badge '+task.state},sending?'发送中':states[task.state]||task.state)]),
   h('div',{class:'schedule-task-grid'},[
    h('div',null,[h('span',{class:'schedule-label'},'发信设备'),h('strong',null,this.deviceName(task.device_id))]),
    h('div',null,[h('span',{class:'schedule-label'},'收信号码'),h('strong',null,task.phone)]),
    h('div',null,[h('span',{class:'schedule-label'},'执行规则'),h('strong',null,task.mode==='once'?'指定时间 · 仅一次':intervalText(task))]),
    h('div',null,[h('span',{class:'schedule-label'},'下次执行时间'),h('strong',{class:task.next_run?'schedule-next':''},sending?'正在执行':formatTime(task.next_run))])]),
   h('p',{class:'schedule-message',title:task.message},task.message),
   h('div',{class:'schedule-task-bottom'},[h('div',{class:'schedule-result'},[h('span',null,task.last_result||'等待开始'),task.last_run?h('small',null,`最近执行 ${formatTime(task.last_run)} · 已提交 ${task.run_count} 次`):null]),
    h('div',{class:'schedule-actions'},[this.button(task.state==='active'?'暂停':'开始',()=>this.action(task,task.state==='active'?'pause':'start'),task.state==='active'?'':'primary',{disabled:this.actionId===task.id||task.state==='completed'}),this.button('修改',()=>this.open(task),'',{disabled:sending}),this.button('记录',()=>this.showHistory(task)),this.button('删除',()=>this.confirmDelete(task),'danger',{disabled:sending})])])
  ]));}
  blocks.push(h('p',{class:'schedule-footnote'},'新建或修改任务后默认暂停。错过的执行时间不集中补发；发送失败或结果不确定时自动暂停，详情可查看执行记录。'));
  if(this.form){const f=this.form;blocks.push(this.dialog(this.editing?'修改任务':'新增任务',[
   h('form',{id:'schedule-form',onSubmit:e=>{e.preventDefault();this.save();}},[
    this.field('任务名称',this.input('name',{ref:'taskName',required:true,maxlength:80,placeholder:'例如：每日提醒'})),
    h('div',{class:'schedule-form-row'},[this.field('发信设备',h('select',{value:f.device_id,onChange:e=>f.device_id=e.target.value,required:true},[h('option',{value:'',disabled:true},'请选择设备'),...this.devices.map(d=>h('option',{value:d.id},`${d.name} · ${d.running?'在线':'离线'}`))])),this.field('收信号码',this.input('phone',{type:'tel',required:true,maxlength:24,placeholder:'例如：+8613800138000'}))]),
    this.field('短信内容',h('textarea',{value:f.message,onInput:e=>f.message=e.target.value,rows:4,maxlength:2000,required:true,placeholder:'输入要发送的短信内容'}),`${f.message.length} / 2000 字符，长短信可能分为多条计费`),
    h('div',{class:'schedule-mode','aria-label':'执行方式'},[['once','指定时间 · 发送一次'],['interval','按间隔 · 重复发送']].map(([value,label])=>this.button(label,()=>f.mode=value,f.mode===value?'selected':'',{'aria-pressed':f.mode===value}))),
    f.mode==='interval'?h('div',{class:'schedule-interval'},[['days','天',3659],['hours','小时',23],['minutes','分钟',59],['seconds','秒',59]].map(([key,label,max])=>this.field(label,this.input(key,{type:'number',min:0,max,step:1,required:true})))):null,
    this.field(f.mode==='once'?'执行日期和时间':'首次执行日期和时间',this.input('first',{type:'datetime-local',step:1,required:true}), '北京时间（UTC+8），可精确到秒'),
    h('div',{class:'schedule-hint'},'保存后任务为暂停状态。点击列表中的“开始”后才会按时发送。'),
    this.formError?h('div',{class:'schedule-alert error',role:'alert'},this.formError):null
   ])],[this.button('取消',()=>this.closeDialog(),'',{disabled:this.busy}),h('button',{type:'submit',form:'schedule-form',class:'schedule-button primary',disabled:this.busy},this.busy?'保存中…':'保存任务')]));}
  if(this.deleteTask)blocks.push(this.dialog('删除任务',[h('p',null,`确定删除“${this.deleteTask.name}”及其执行记录吗？`),h('p',{class:'schedule-muted'},'短信中心中已有的短信记录将保留。'),this.formError?h('p',{role:'alert'},this.formError):null],[this.button('取消',()=>this.closeDialog(),'',{ref:'cancelDelete',disabled:this.busy}),this.button(this.busy?'删除中…':'删除任务',()=>this.remove(),'danger',{disabled:this.busy})]));
  if(this.historyTask)blocks.push(this.dialog('执行记录 · '+this.historyTask.name,[this.historyError?h('p',{role:'alert'},this.historyError):null,...(this.runs.length?this.runs.map(r=>h('div',{class:'schedule-run'},[h('strong',null,formatTime(r.scheduled_for)),h('span',null,({success:'已提交',failed:'失败',unknown:'结果未知',sending:'发送中',skipped:'已跳过'})[r.status]||r.status),h('p',null,r.detail)])):[h('p',{class:'schedule-muted'},'暂无执行记录')])],[this.button('关闭',()=>this.closeDialog(),'schedule-close',{'aria-label':'关闭'})]));
  return h('section',{class:'schedule-page'},blocks);
 }
};
