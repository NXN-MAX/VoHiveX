// Images stay inside this function. No upload, object URL, storage or image preview.
export async function decodeQrFile(input, decoder) {
 let file=input.files?.[0], bitmap=null, canvas=null, pixels=null, result=null;
 input.value='';
 try {
  if(!file)throw Error('请选择二维码图片');
  if(!['image/png','image/jpeg','image/webp'].includes(file.type)||file.size>8*1024*1024)throw Error('请选择 8 MB 以内的 PNG、JPG 或 WebP 图片');
  bitmap=await createImageBitmap(file);file=null;
  if(bitmap.width*bitmap.height>16000000)throw Error('图片过大，请裁剪为二维码后重新选择');
  canvas=document.createElement('canvas');canvas.width=bitmap.width;canvas.height=bitmap.height;
  const context=canvas.getContext('2d',{willReadFrequently:true});
  context.drawImage(bitmap,0,0);pixels=context.getImageData(0,0,canvas.width,canvas.height);
  result=decoder(pixels.data,pixels.width,pixels.height,{inversionAttempts:'attemptBoth'});
  if(!result?.data)throw Error('未识别到二维码，请使用清晰完整的二维码图片');
  const text=result.data.trim();
  if(text.length>65536||! /^(https:\/\/|vmess:\/\/|vless:\/\/|ss:\/\/|trojan:\/\/|anytls:\/\/|hy2:\/\/|hysteria2:\/\/|tuic:\/\/|socks5:\/\/)/i.test(text))throw Error('二维码不是支持的订阅地址或节点链接');
  return text;
 } finally {
  input.value='';file=null;
  if(result){result.data='';result.binaryData?.fill(0);result.chunks?.splice(0);result=null;}
  pixels?.data.fill(0);pixels=null;
  if(canvas){canvas.getContext('2d')?.clearRect(0,0,canvas.width,canvas.height);canvas.width=0;canvas.height=0;canvas.remove();canvas=null;}
  bitmap?.close();bitmap=null;
 }
}
