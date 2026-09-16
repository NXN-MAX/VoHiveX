// Split the existing settings renderer without changing notification operations.
const fs=require('fs'),acorn=require('../build-tools/node_modules/acorn');
const path=process.argv[2];let source=fs.readFileSync(path,'utf8');
const edits=[];
function walk(node){
 if(!node||typeof node!=='object')return;
 if(node.type==='CallExpression'&&node.callee.name==='o'&&node.arguments[1]?.type==='Identifier'){
  const name=node.arguments[1].name;
  if(['te','pe','me'].includes(name)){
   const code=name==='pe'?'o(g,null,[o("section",{class:"ui-card settings-version-card"},[r(VersionInformation)]),o("section",pe,[r(SystemInformation)])])':source.slice(node.start,node.end);
   edits.push([node.start,node.end,`(${name==='me'?'settingsProps.pushOnly':'!settingsProps.pushOnly'}?${code}:u("",true))`]);
   return;
  }
 }
 for(const value of Object.values(node))if(Array.isArray(value))value.forEach(walk);else if(value&&typeof value==='object')walk(value);
}
walk(acorn.parse(source,{ecmaVersion:'latest',sourceType:'module'}));
if(edits.length!==3)throw Error('Expected security, docs and notification sections');
for(const [start,end,text]of edits.sort((a,b)=>b[0]-a[0]))source=source.slice(0,start)+text+source.slice(end);
fs.writeFileSync(path,source);
