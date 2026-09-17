import {X as h} from '/assets/vendor-DJI26078.js';

export default {
 render(){
  const row=(label,value)=>h('div',{class:'system-info-row'},[h('dt',null,typeof label==='string'?label:[label]),h('dd',null,typeof value==='string'?value:[value])]);
  const author=(name)=>h('a',{href:'https://github.com/'+name,target:'_blank',rel:'noopener noreferrer'},name);
  return h('div',{class:'version-information'},[
   h('h3',null,'版本信息'),
   h('dl',null,[row('原作者',author('iniwex5')),row(h('span',null,['VoHive',h('span',{class:'vh-brand-x'},'X'),'作者']),author('NXN-MAX')),row('版本号','2.0.2')])
  ]);
 }
};
