// Replace only verified SVG component initializers; preserve exports and handlers.
const fs = require('fs');
const acorn = require('../build-tools/node_modules/acorn');
const [file, family, version] = process.argv.slice(2);
const map = JSON.parse(fs.readFileSync(__dirname + '/remix-map.json', 'utf8'))[family];
const source = fs.readFileSync(file, 'utf8');
const ast = acorn.parse(source, {ecmaVersion:'latest', sourceType:'module'});
const edits = [], seen = new Set();
function walk(node) {
  if (!node || typeof node !== 'object') return;
  if (node.type === 'VariableDeclarator' && node.init?.type === 'CallExpression') {
    const obj = node.init.arguments[0];
    if (obj?.type === 'ObjectExpression') {
      const name = obj.properties.find(p => p.key?.name === 'name')?.value?.value;
      if (map[name]) {
        if (seen.has(name) || !source.slice(node.init.start,node.init.end).includes('"svg"')) throw Error('Not a unique SVG: '+name);
        seen.add(name);
        edits.push({start:node.init.start,end:node.init.end,text:'Remix.'+name});
      }
    }
  }
  for (const value of Object.values(node)) {
    if (Array.isArray(value)) value.forEach(walk);
    else if (value && typeof value === 'object') walk(value);
  }
}
walk(ast);
for (const name of Object.keys(map)) if (!seen.has(name)) throw Error('Missing icon '+name);
let output=source;
for (const edit of edits.sort((a,b)=>b.start-a.start)) output=output.slice(0,edit.start)+edit.text+output.slice(edit.end);
output='import {Remix} from "./RemixIcons-'+version+'.js";'+output;
acorn.parse(output,{ecmaVersion:'latest',sourceType:'module'});
fs.writeFileSync(file,output);
console.log(family+': '+edits.length+' Remix icons');
