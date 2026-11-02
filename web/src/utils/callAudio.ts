import { ref } from 'vue'
import { request } from '@/api/http'

export function useCallAudio() {
  const connected = ref(false), busy = ref(false), muted = ref(false)
  const inputLevel = ref(0), outputLevel = ref(0), error = ref('')
  let ws: WebSocket | null = null, context: AudioContext | null = null
  let node: AudioWorkletNode | null = null, mic: MediaStream | null = null
  let generation = 0
  function disconnect() {
    generation++; connected.value = false
    if (ws) { ws.onclose = null; ws.close(); ws = null }
    mic?.getTracks().forEach((t) => t.stop()); mic = null
    node?.disconnect(); node = null
    if (context) void context.close(); context = null
    inputLevel.value = 0; outputLevel.value = 0
  }
  function setMuted(value: boolean) { muted.value = value; node?.port.postMessage({ type: 'mute', value }) }
  async function connect(deviceID: string, listenOnly: boolean) {
    disconnect(); const own = generation; busy.value = true; error.value = ''
    try {
      if (!window.isSecureContext) throw new Error('网页音频需要 HTTPS 或 localhost 安全连接')
      // 48 kHz gives an exact 6:1 microphone conversion to modem PCM.
      context = new AudioContext({ sampleRate: 48000, latencyHint: 'interactive' })
      await context.resume()
      await context.audioWorklet.addModule('/worklets/call-audio.js')
      if (!listenOnly) {
        const stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true }, video: false })
        if (own !== generation) { stream.getTracks().forEach((t) => t.stop()); return }
        mic = stream
      }
      if (own !== generation || !context) return
      node = new AudioWorkletNode(context, 'vohivex-call-audio', { numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [1] })
      node.connect(context.destination)
      if (mic) context.createMediaStreamSource(mic).connect(node)
      setMuted(listenOnly)
      const ticket = await request<{ ticket: string }>({ method: 'POST', url: '/calls/audio/ticket', data: { device_id: deviceID } })
      if (own !== generation) return
      const socket = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/calls/audio/stream`)
      ws = socket; socket.binaryType = 'arraybuffer'
      await new Promise<void>((resolve, reject) => {
        const timer = window.setTimeout(() => reject(new Error('网页音频连接超时')), 8000)
        const fail = () => { window.clearTimeout(timer); reject(new Error('音频连接已断开，请重新连接')) }
        socket.onopen = () => socket.send(JSON.stringify({ ticket: ticket.ticket }))
        socket.onerror = fail
        socket.onclose = () => { fail(); if (own === generation) { disconnect(); error.value = '网页音频已断开，电话不会自动挂断' } }
        socket.onmessage = ({ data }) => {
          if (typeof data === 'string') {
            try { if (JSON.parse(data).ready) { window.clearTimeout(timer); connected.value = true; resolve() } } catch { fail() }
            return
          }
          const view = new DataView(data as ArrayBuffer), samples = new Float32Array(view.byteLength / 2)
          for (let i = 0; i < samples.length; i++) samples[i] = view.getInt16(i * 2, true) / 32768
          node?.port.postMessage({ type: 'pcm', samples }, [samples.buffer])
        }
      })
      if (own !== generation || !node) return
      node.port.onmessage = ({ data }) => {
        if (data.type === 'level') outputLevel.value = Math.min(1, data.outputPeak)
        if (data.type === 'pcm' && socket.readyState === WebSocket.OPEN) {
          inputLevel.value = Math.min(1, data.inputPeak)
          if (socket.bufferedAmount > 32_000) { disconnect(); error.value = '音频网络拥塞，请重新连接'; return }
          socket.send(data.buffer)
        }
      }
    } catch (reason) {
      if (own === generation) {
        disconnect()
        const detail = reason instanceof DOMException
          ? ({ NotAllowedError: '麦克风权限未授予，可允许麦克风或选择仅收听', NotFoundError: '没有可用麦克风，可连接麦克风或选择仅收听', NotReadableError: '麦克风被占用或无法打开，请检查系统音频设置' } as Record<string, string>)[reason.name]
          : ''
        error.value = detail || (reason instanceof Error ? reason.message : '无法连接网页音频')
        throw new Error(error.value)
      }
    } finally { busy.value = false }
  }
  return { connected, busy, muted, inputLevel, outputLevel, error, connect, disconnect, setMuted }
}
