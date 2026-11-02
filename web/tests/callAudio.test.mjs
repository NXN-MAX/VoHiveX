import { test } from 'node:test'
import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFileSync } from 'node:fs'
const source = readFileSync(new URL('../public/worklets/call-audio.js', import.meta.url), 'utf8')
function processor(rate) {
 const events=[];let Processor
 vm.runInNewContext(source,{ sampleRate:rate, Float32Array, ArrayBuffer, DataView,
 AudioWorkletProcessor:class {constructor(){this.port={postMessage:(event)=>events.push(event)}}},
 registerProcessor:(_name, value)=>Processor=value })
 return {p:new Processor(), events}
}
for (const rate of [44100,48000]) test(`microphone produces 8 kHz PCM at ${rate} Hz without local echo`,()=>{
 const {p,events}=processor(rate)
 for(let n=0;n<rate;n+=128){const size=Math.min(128,rate-n),input=new Float32Array(size).fill(0.5),output=new Float32Array(size);p.process([[input]],[[output]]);assert.ok(output.every(n=>n===0))}
 const frames=events.filter(e=>e.type==='pcm');assert.equal(frames.length,50)
 for(const frame of frames){assert.equal(frame.buffer.byteLength,320);assert.equal(new DataView(frame.buffer).getInt16(0,true),16384)}
})
test('muting sends silence and downlink remains audible',()=>{
 const {p,events}=processor(48000);p.port.onmessage({data:{type:'mute',value:true}});p.port.onmessage({data:{type:'pcm',samples:new Float32Array(1600).fill(0.25)}})
 for(let n=0;n<1024;n+=128){const out=new Float32Array(128);p.process([[new Float32Array(128).fill(0.7)]],[[out]]);assert.ok(out.every(n=>n===0.25))}
 const frame=events.find(e=>e.type==='pcm');assert.ok(new Uint8Array(frame.buffer).every(n=>n===0))
})
test('a stalled downlink discards old buffered speech',()=>{
 const {p}=processor(48000)
 for(let n=0;n<100;n++)p.port.onmessage({data:{type:'pcm',samples:new Float32Array(1600)}})
 assert.ok(p.write-p.read<=4000)
})
