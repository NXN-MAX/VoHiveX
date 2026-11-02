// Browser microphone -> 8 kHz PCM; remote PCM -> browser output rate.
// Bounded queues prevent stale speech from playing after a stalled connection.
class CallAudio extends AudioWorkletProcessor {
  constructor() {
    super()
    this.ratio = sampleRate / 8000
    this.sum = 0; this.weight = 0; this.frame = []; this.muted = false
    this.rx = new Float32Array(8000); this.read = 0; this.write = 0; this.phase = 0
    this.levelFrames = 0; this.levelPeak = 0
    this.port.onmessage = ({ data }) => {
      if (data.type === 'mute') { this.muted = data.value; return }
      if (data.type !== 'pcm' || !(data.samples instanceof Float32Array)) return
      if (this.write - this.read + data.samples.length > 4000) { this.read = this.write; this.phase = 0 }
      for (const value of data.samples) this.rx[(this.write++) % this.rx.length] = value
    }
  }
  process(inputs, outputs) {
    const mic = inputs[0]?.[0], out = outputs[0]?.[0]
    if (!out) return true
    let inputPeak = 0, outputPeak = 0
    for (let i = 0; i < out.length; i++) {
      const value = this.muted ? 0 : (mic?.[i] || 0)
      inputPeak = Math.max(inputPeak, Math.abs(value))
      let remaining = 1
      while (remaining > 1e-8) {
        const weight = Math.min(remaining, this.ratio - this.weight)
        this.sum += value * weight; this.weight += weight; remaining -= weight
        if (this.weight >= this.ratio - 1e-8) {
          this.frame.push(Math.max(-32768, Math.min(32767, Math.round(this.sum / this.ratio * 32767))))
          this.sum = 0; this.weight = 0
          if (this.frame.length === 160) {
            const b = new ArrayBuffer(320), v = new DataView(b)
            this.frame.forEach((n, j) => v.setInt16(j * 2, n, true))
            this.port.postMessage({ type: 'pcm', buffer: b, inputPeak }, [b]); this.frame = []
          }
        }
      }
      let played = 0
      if (this.write - this.read >= 2) {
        const a = this.rx[this.read % this.rx.length], b = this.rx[(this.read + 1) % this.rx.length]
        played = a + (b - a) * this.phase
        this.phase += 1 / this.ratio
        if (this.phase >= 1) { const n = Math.floor(this.phase); this.read += n; this.phase -= n }
      }
      out[i] = played; outputPeak = Math.max(outputPeak, Math.abs(played))
    }
    this.levelFrames += out.length; this.levelPeak = Math.max(this.levelPeak, outputPeak)
    if (this.levelFrames >= sampleRate / 20) { this.port.postMessage({ type: 'level', outputPeak: this.levelPeak }); this.levelFrames = 0; this.levelPeak = 0 }
    return true
  }
}
registerProcessor('vohivex-call-audio', CallAudio)
