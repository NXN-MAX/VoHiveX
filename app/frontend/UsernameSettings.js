import {X as h} from '/assets/vendor-DJI26055.js';
import {b as api} from '/assets/route-sms-DJI26055.js';
export default {
 data:()=>({current:'',username:'',password:'',loading:true,busy:false,error:'',done:false}),
 async mounted(){try{const r=await api.get('/settings/username');this.current=r.data.username;this.username=this.current}catch{this.error='用户名加载失败，请刷新后重试'}finally{this.loading=false}},
 methods:{async save(){
  if(this.busy||this.done)return;this.error='';
  if(!/^[A-Za-z0-9_.@-]{3,64}$/.test(this.username)){this.error='用户名需为 3–64 位字母、数字或 _ . @ -';return}
  if(this.username===this.current){this.error='请填写不同的新用户名';return}
  if(!this.password){this.error='请填写当前密码';return}
  this.busy=true;
  try{await api.post('/settings/username',{username:this.username,current_password:this.password});this.done=true;this.password='';localStorage.removeItem('token');
   let attempts=0;const reconnect=async()=>{try{const r=await fetch('/',{cache:'no-store'});if(r.ok){location.replace('/?account-updated=1#/login');return}}catch{}if(++attempts<30)setTimeout(reconnect,2000);else this.error='用户名已保存，请稍后刷新页面重新登录'};setTimeout(reconnect,5000)
  }
  catch(e){this.error=e.response?.data?.error||'修改失败，请稍后重试'}finally{this.busy=false}
 }},
 render(){return h('form',{class:'username-settings',onSubmit:e=>{e.preventDefault();this.save()}},[
  h('header',null,[h('h3',null,'用户名修改'),h('p',{class:'username-hint'},'更新登录账号')]),
  h('label',{for:'account-username'},'用户名'),h('input',{id:'account-username',autocomplete:'username',value:this.username,disabled:this.loading||this.busy||this.done,maxlength:64,onInput:e=>this.username=e.target.value}),
  h('label',{for:'account-current-password'},'当前密码'),h('input',{id:'account-current-password',type:'password',autocomplete:'current-password',value:this.password,disabled:this.busy||this.done,onInput:e=>this.password=e.target.value}),
  h('p',{class:'username-hint'},'修改后服务会短暂重启，请使用新用户名和原密码重新登录。'),
  this.error?h('p',{class:'username-error',role:'alert'},this.error):null,
  this.done?h('p',{role:'status'},'用户名已保存，正在重新启动服务…'):null,
  h('button',{type:'submit',class:'el-button el-button--primary',disabled:this.loading||this.busy||this.done},this.busy?'保存中…':'保存用户名')
 ])}
};
