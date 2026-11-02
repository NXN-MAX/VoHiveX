#include "audio_darwin.h"
#include <CoreAudio/CoreAudio.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <math.h>
#define CAP 8000
typedef struct {int16_t data[CAP];atomic_uint read,write;} ring;
static ring rx,tx;
static AudioDeviceID inputDevice,outputDevice;
static AudioDeviceIOProcID inputProc,outputProc;
static int push(ring*r,const int16_t*p,int n){unsigned w=atomic_load(&r->write),rd=atomic_load(&r->read);if(n>CAP-(w-rd))n=CAP-(w-rd);for(int i=0;i<n;i++)r->data[(w+i)%CAP]=p[i];atomic_store(&r->write,w+n);return n;}
static int pop(ring*r,int16_t*p,int n){unsigned rd=atomic_load(&r->read),w=atomic_load(&r->write);if(n>w-rd)n=w-rd;for(int i=0;i<n;i++)p[i]=r->data[(rd+i)%CAP];atomic_store(&r->read,rd+n);return n;}
int vx_audio_read(int16_t*p,int n){return pop(&rx,p,n);}
int vx_audio_write(const int16_t*p,int n){return push(&tx,p,n);}
static OSStatus capture(AudioDeviceID d,const AudioTimeStamp*t,const AudioBufferList*in,const AudioTimeStamp*it,AudioBufferList*out,const AudioTimeStamp*ot,void*x){
 if(in&&in->mNumberBuffers&&in->mBuffers[0].mData){const float*p=in->mBuffers[0].mData;unsigned n=in->mBuffers[0].mDataByteSize/4;int16_t b[1024];for(unsigned at=0;at<n;){unsigned len=n-at;if(len>1024)len=1024;for(unsigned i=0;i<len;i++){float v=isfinite(p[at+i])?p[at+i]:0;b[i]=(int16_t)lrintf(fmaxf(-1,fminf(1,v))*32767);}push(&rx,b,len);at+=len;}}return noErr;
}
static OSStatus playback(AudioDeviceID d,const AudioTimeStamp*t,const AudioBufferList*in,const AudioTimeStamp*it,AudioBufferList*out,const AudioTimeStamp*ot,void*x){
 if(out)for(UInt32 j=0;j<out->mNumberBuffers;j++){float*p=out->mBuffers[j].mData;unsigned n=out->mBuffers[j].mDataByteSize/4;int16_t b[1024];for(unsigned at=0;p&&at<n;){unsigned len=n-at;if(len>1024)len=1024;int got=pop(&tx,b,len);for(unsigned i=0;i<len;i++)p[at+i]=i<got?b[i]/32768.0f:0;at+=len;}}return noErr;
}
static int fmt(AudioDeviceID d,UInt32 scope){AudioObjectPropertyAddress a={kAudioDevicePropertyStreamFormat,scope,kAudioObjectPropertyElementMain};AudioStreamBasicDescription f={0};UInt32 z=sizeof(f);return !AudioObjectGetPropertyData(d,&a,0,NULL,&z,&f)&&f.mSampleRate==8000&&f.mFormatID==kAudioFormatLinearPCM&&(f.mFormatFlags&kAudioFormatFlagIsFloat)&&!(f.mFormatFlags&kAudioFormatFlagIsBigEndian)&&f.mChannelsPerFrame==1&&f.mBitsPerChannel==32&&f.mBytesPerFrame==4;}
int vx_audio_start(uint32_t location){
 char prefix[256];snprintf(prefix,sizeof(prefix),"AppleUSBAudioEngine:BAIWANG:Baiwang:%x:",location);
 AudioObjectPropertyAddress a={kAudioHardwarePropertyDevices,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain};UInt32 z=0;OSStatus err=AudioObjectGetPropertyDataSize(kAudioObjectSystemObject,&a,0,NULL,&z);if(err)return err;
 AudioDeviceID*ids=malloc(z);if(!ids)return -1;err=AudioObjectGetPropertyData(kAudioObjectSystemObject,&a,0,NULL,&z,ids);if(err){free(ids);return err;}
 int ni=0,no=0;for(unsigned i=0;i<z/sizeof(*ids);i++){a=(AudioObjectPropertyAddress){kAudioDevicePropertyDeviceUID,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain};CFStringRef s=NULL;UInt32 q=sizeof(s);char uid[512]={0};if(!AudioObjectGetPropertyData(ids[i],&a,0,NULL,&q,&s)&&s){CFStringGetCString(s,uid,sizeof(uid),kCFStringEncodingUTF8);CFRelease(s);if(!strncmp(uid,prefix,strlen(prefix))){if(fmt(ids[i],kAudioDevicePropertyScopeInput)){inputDevice=ids[i];ni++;}if(fmt(ids[i],kAudioDevicePropertyScopeOutput)){outputDevice=ids[i];no++;}}}}
 free(ids);if(ni!=1||no!=1)return -2;
 atomic_store(&rx.read,0);atomic_store(&rx.write,0);atomic_store(&tx.read,0);atomic_store(&tx.write,0);
 err=AudioDeviceCreateIOProcID(inputDevice,capture,NULL,&inputProc);if(!err)err=AudioDeviceCreateIOProcID(outputDevice,playback,NULL,&outputProc);if(!err)err=AudioDeviceStart(outputDevice,outputProc);if(!err)err=AudioDeviceStart(inputDevice,inputProc);if(err)vx_audio_stop();return err;
}
void vx_audio_stop(void){if(inputProc){AudioDeviceStop(inputDevice,inputProc);AudioDeviceDestroyIOProcID(inputDevice,inputProc);inputProc=NULL;}if(outputProc){AudioDeviceStop(outputDevice,outputProc);AudioDeviceDestroyIOProcID(outputDevice,outputProc);outputProc=NULL;}}
