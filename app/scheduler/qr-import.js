// Images stay inside this function. No upload, object URL, storage or image preview.
async function decodeQrImage(source, decoder) {
 const input=source?.files?source:null;
 let file=input?input.files?.[0]:source, bitmap=null, canvas=null, pixels=null, result=null;
 if(input)input.value='';
 try {
  if(!file)throw Error('请选择二维码图片');
  if(!['image/png','image/jpeg','image/webp'].includes(file.type)||file.size>8*1024*1024)throw Error('请选择 8 MB 以内的 PNG、JPG 或 WebP 图片');
  if(typeof decoder!=='function')throw Error('二维码识别组件尚未加载，请稍后重试');
  bitmap=await createImageBitmap(file);file=null;
  if(bitmap.width*bitmap.height>16000000)throw Error('图片过大，请裁剪为二维码后重新选择');
  canvas=document.createElement('canvas');canvas.width=bitmap.width;canvas.height=bitmap.height;
  const context=canvas.getContext('2d',{willReadFrequently:true});
  context.drawImage(bitmap,0,0);pixels=context.getImageData(0,0,canvas.width,canvas.height);
  result=decoder(pixels.data,pixels.width,pixels.height,{inversionAttempts:'attemptBoth'});
  if(!result?.data)throw Error('未识别到二维码，请使用清晰完整的二维码图片');
  const text=result.data.trim();
  if(!text||text.length>65536)throw Error('二维码内容为空或过长');
  return text;
 } finally {
  if(input)input.value='';file=null;
  if(result){result.data='';result.binaryData?.fill(0);result.chunks?.splice(0);result=null;}
  pixels?.data.fill(0);pixels=null;
  if(canvas){canvas.getContext('2d')?.clearRect(0,0,canvas.width,canvas.height);canvas.width=0;canvas.height=0;canvas.remove();canvas=null;}
  bitmap?.close();bitmap=null;
 }
}

export async function decodeQrFile(input, decoder) {
 const text=await decodeQrImage(input,decoder);
 if(! /^(https:\/\/|vmess:\/\/|vless:\/\/|ss:\/\/|trojan:\/\/|anytls:\/\/|hy2:\/\/|hysteria2:\/\/|tuic:\/\/|socks5:\/\/)/i.test(text))throw Error('二维码不是支持的订阅地址或节点链接');
 return text;
}

export function parseLpaActivationCode(value) {
 const text=String(value||'').trim();
 if(!text||text.length>2048)throw Error('请输入有效的 eSIM 激活链接');
 if(/[\u0000-\u001f\u007f]/.test(text))throw Error('eSIM 激活链接包含无效字符');
 const parts=text.split('$');
 if(parts.length<3||parts.length>4||parts[0].toUpperCase()!=='LPA:1')throw Error('链接格式应为 LPA:1$SM-DP+地址$Matching ID（可空）$确认码（可选）');
 const smdp=parts[1].trim(),matchingId=parts[2].trim(),confirmationCode=(parts[3]||'').trim();
 const hostLabel='[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?';
 const smdpPattern=new RegExp('^(?=.{1,255}$)(?:'+hostLabel+'\\.)+'+hostLabel+'(?::(?:[1-9][0-9]{0,4}))?$');
 if(!smdpPattern.test(smdp))throw Error('SM-DP+ 地址格式不正确');
 if(matchingId&&(matchingId.length>255||!/^[0-9A-Z-]+$/.test(matchingId)))throw Error('Matching ID 只能包含大写字母、数字和连字符');
 if(confirmationCode&&(confirmationCode.length>255||!/^[\x21-\x23\x25-\x7e]+$/.test(confirmationCode)))throw Error('确认码格式不正确');
 return {smdp,matchingId,confirmationCode};
}

export async function decodeEsimQrFile(file, decoder) {
 const text=await decodeQrImage(file,decoder);
 parseLpaActivationCode(text);
 return text;
}
