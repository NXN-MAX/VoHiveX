const FIELDS = ['device_id','device_name','local_phone','imsi','peer','message_id','direction','sender','content','timestamp','status'];
const ALIASES = {
 device_id:['device_id','device','设备ID'],device_name:['device_name','设备名称'],local_phone:['local_phone','device_phone','本机号码','设备号码'],
 imsi:['imsi'],peer:['peer','contact','phone','联系人','对方号码'],message_id:['message_id','id','短信ID'],
 direction:['direction','type','方向'],sender:['sender','发送方'],content:['content','message','text','短信内容','内容'],
 timestamp:['timestamp','time','date','时间'],status:['status','状态']
};

function clean(value){return String(value??'').replace(/^\uFEFF/,'').trim();}
function pick(row,key){for(const name of ALIASES[key])if(row[name]!==undefined)return row[name];return '';}
function direction(value){const text=clean(value).toLowerCase();return ['2','sent','outgoing','send','已发送','发送'].includes(text)?2:1;}
function status(value){const text=clean(value).toLowerCase();if(!text)return null;if(['success','sent','delivered','成功','已发送'].includes(text))return 2;if(['failed','error','失败'].includes(text))return 3;const number=Number(text);return Number.isInteger(number)&&number>=0&&number<=3?number:null;}

export function normalizeMessages(rows){
 if(!Array.isArray(rows))throw Error('短信文件格式无效');
 const output=[];
 for(const source of rows){
  if(!source||typeof source!=='object')continue;
  const peer=clean(pick(source,'peer')),content=String(pick(source,'content')??''),timestamp=clean(pick(source,'timestamp'));
  if(!peer||!content||!timestamp||!Number.isFinite(Date.parse(timestamp)))continue;
  output.push({device_id:clean(pick(source,'device_id')),device_name:clean(pick(source,'device_name')),local_phone:clean(pick(source,'local_phone')),imsi:clean(pick(source,'imsi')),peer,message_id:clean(pick(source,'message_id')),type:direction(pick(source,'direction')),sender:clean(pick(source,'sender')),content,timestamp:new Date(timestamp).toISOString(),status:status(pick(source,'status'))});
 }
 if(!output.length)throw Error('文件中没有可识别的短信记录');
 if(output.length>20000)throw Error('一次最多导入 20000 条短信');
 return output;
}

export function parseDelimited(text,delimiter=','){
 const rows=[];let row=[],field='',quoted=false;
 const source=String(text??'').replace(/^\uFEFF/,'');
 for(let index=0;index<source.length;index++){
  const char=source[index];
  if(quoted){if(char==='"'&&source[index+1]==='"'){field+='"';index++;}else if(char==='"')quoted=false;else field+=char;continue;}
  if(char==='"'){quoted=true;continue;}
  if(char===delimiter){row.push(field);field='';continue;}
  if(char==='\n'){row.push(field);rows.push(row);row=[];field='';continue;}
  if(char!=='\r')field+=char;
 }
 row.push(field);if(row.some(value=>value!==''))rows.push(row);
 if(rows.length<2)throw Error('文件中没有短信数据');
 const headers=rows.shift().map(clean);
 return normalizeMessages(rows.filter(values=>values.some(Boolean)).map(values=>Object.fromEntries(headers.map((header,index)=>[header,values[index]??'']))));
}

function escapeXml(value){return String(value??'').replace(/[<>&"']/g,char=>({'<':'&lt;','>':'&gt;','&':'&amp;','"':'&quot;',"'":'&apos;'}[char]));}
function escapeHtml(value){return escapeXml(value);}
function fieldValue(row,field){if(field==='direction')return Number(row.type)===2?'sent':'received';return row[field]??'';}
function csvCell(value){const text=String(value??'');return /[",\r\n]/.test(text)?'"'+text.replace(/"/g,'""')+'"':text;}

export function serializeMessages(rows,format){
 if(!Array.isArray(rows)||!rows.length)throw Error('没有可导出的短信');
 const normalized=rows.map(row=>Object.fromEntries(FIELDS.map(field=>[field,fieldValue(row,field)])));
 if(format==='csv')return '\uFEFF'+[FIELDS.join(','),...normalized.map(row=>FIELDS.map(field=>csvCell(row[field])).join(','))].join('\r\n');
 if(format==='txt')return [FIELDS.join('\t'),...normalized.map(row=>FIELDS.map(field=>String(row[field]??'').replace(/[\t\r\n]+/g,' ')).join('\t'))].join('\n');
 if(format==='html')return '<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>VoHiveX 短信归档</title></head><body><table><thead><tr>'+FIELDS.map(field=>'<th>'+field+'</th>').join('')+'</tr></thead><tbody>'+normalized.map(row=>'<tr>'+FIELDS.map(field=>'<td>'+escapeHtml(row[field])+'</td>').join('')+'</tr>').join('')+'</tbody></table></body></html>';
 if(format==='xml')return '<?xml version="1.0" encoding="UTF-8"?>\n<smsArchive version="1">\n'+normalized.map(row=>'  <message>'+FIELDS.map(field=>`<${field}>${escapeXml(row[field])}</${field}>`).join('')+'</message>').join('\n')+'\n</smsArchive>';
 throw Error('不支持的导出格式');
}

export function parseArchive(text,format,domParser=globalThis.DOMParser){
 if(format==='csv')return parseDelimited(text,',');
 if(format==='txt')return parseDelimited(text,'\t');
 if(format!=='html'&&format!=='xml')throw Error('仅支持 CSV、TXT、HTML、XML');
 if(typeof domParser!=='function')throw Error('当前浏览器无法读取该文件');
 const document=new domParser().parseFromString(String(text??''),format==='html'?'text/html':'application/xml');
 if(document.querySelector('parsererror'))throw Error('文件结构无效');
 if(format==='html'){
  const table=document.querySelector('table');if(!table)throw Error('HTML 中没有短信表格');
  const headers=[...table.querySelectorAll('thead th')].map(cell=>clean(cell.textContent));
  const rows=[...table.querySelectorAll('tbody tr')].map(tr=>Object.fromEntries([...tr.querySelectorAll('td')].map((cell,index)=>[headers[index]||'',cell.textContent||''])));
  return normalizeMessages(rows);
 }
 const rows=[...document.querySelectorAll('message')].map(node=>Object.fromEntries(FIELDS.map(field=>[field,node.querySelector(field)?.textContent||''])));
 return normalizeMessages(rows);
}

export function conversationKey(row){return `${row.imsi||''}|${row.peer||''}`;}
export function groupConversations(rows){
 const groups=new Map;
 for(const row of rows){const key=conversationKey(row);if(!groups.has(key))groups.set(key,{key,imsi:row.imsi||'',peer:row.peer,count:0,lastTimestamp:row.timestamp});const group=groups.get(key);group.count++;if(Date.parse(row.timestamp)>Date.parse(group.lastTimestamp))group.lastTimestamp=row.timestamp;}
 return [...groups.values()].sort((a,b)=>Date.parse(b.lastTimestamp)-Date.parse(a.lastTimestamp));
}
