"use strict";

const NATIVE_AUDIO_TAPS = 32;
const NATIVE_AUDIO_PHASES = 1024;
const NATIVE_AUDIO_HISTORY = 15;
const NATIVE_AUDIO_LOOKAHEAD = 16;
const NATIVE_AUDIO_KERNEL = (() => {
  const table = new Float32Array((NATIVE_AUDIO_PHASES + 1) * NATIVE_AUDIO_TAPS);
  const cutoff = 1 / 1.001;
  for (let phase = 0; phase <= NATIVE_AUDIO_PHASES; phase++) {
    let sum = 0;
    const offset = phase * NATIVE_AUDIO_TAPS;
    for (let tap = 0; tap < NATIVE_AUDIO_TAPS; tap++) {
      const position = tap - NATIVE_AUDIO_HISTORY - phase / NATIVE_AUDIO_PHASES;
      const sinc = position === 0 ? cutoff : Math.sin(Math.PI * cutoff * position) / (Math.PI * position);
      const window = Math.abs(position) < 16 ? (1 + Math.cos(Math.PI * position / 16)) / 2 : 0;
      table[offset + tap] = sinc * window;
      sum += table[offset + tap];
    }
    for (let tap = 0; tap < NATIVE_AUDIO_TAPS; tap++) table[offset + tap] /= sum;
  }
  return table;
})();

class LocalDropNativeAudioProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.capacity = 8192;
    this.left = new Float32Array(this.capacity);
    this.right = new Float32Array(this.capacity);
    this.historyLeft = new Float32Array(NATIVE_AUDIO_HISTORY);
    this.historyRight = new Float32Array(NATIVE_AUDIO_HISTORY);
    this.historyWrite = 0;
    this.historyCount = 0;
    this.read = 0;
    this.count = 0;
    this.phase = 0;
    this.started = false;
    this.closed = false;
    this.targetFrames = 2400;
    this.maxFrames = 2880;
    this.averageQueue = this.targetFrames - 240;
    this.rate = 1;
    this.drift = 0;
    this.stableFrames = 0;
    this.reportFrames = 0;
    this.discardedFrames = 0;
    this.underruns = 0;
    this.underrunFrames = 0;
    this.recovering = false;
    this.port.onmessage = ({ data }) => {
      if (this.closed) return;
      if (data?.type === "close") {
        this.closed = true;
        this.count = 0;
        this.phase = 0;
        this.historyCount = this.historyWrite = 0;
        this.port.onmessage = null;
        return;
      }
      if (data?.type !== "pcm" || !(data.buffer instanceof ArrayBuffer) || !data.buffer.byteLength || data.buffer.byteLength > this.capacity * 4 || data.buffer.byteLength % 4) return;
      const pcm = new DataView(data.buffer);
      const frames = pcm.byteLength / 4;
      const drop = Math.min(this.count, Math.max(0, this.count + frames - this.maxFrames));
      this.read = (this.read + drop) % this.capacity;
      this.count -= drop;
      const skip = Math.max(0, frames - this.maxFrames);
      this.discardedFrames += drop + skip;
      if (drop || skip) {
        this.phase = 0;
        this.historyCount = this.historyWrite = 0;
      }
      let write = (this.read + this.count) % this.capacity;
      for (let frame = skip; frame < frames; frame++) {
        this.left[write] = pcm.getInt16(frame * 4, true) / 32768;
        this.right[write] = pcm.getInt16(frame * 4 + 2, true) / 32768;
        write = (write + 1) % this.capacity;
      }
      this.count += frames - skip;
      this.port.postMessage({ type: "accepted", id: data.id, stats: this.stats() });
    };
  }

  stats() {
    return { workletQueueMs: (this.count - this.phase) / 48, targetMs: this.targetFrames / 48, workletDiscardedFrames: this.discardedFrames, underruns: this.underruns, underrunFrames: this.underrunFrames, driftPpm: (this.rate - 1) * 1000000 };
  }

  process(inputs, outputs) {
    const output = outputs[0];
    for (const channel of output) channel.fill(0);
    if (this.closed) return false;
    const length = output[0]?.length || 0;
    this.reportFrames += length;
    if (this.reportFrames >= 4800) {
      this.reportFrames = 0;
      this.port.postMessage({ type: "stats", stats: this.stats() });
    }
    if (!this.started && this.count < this.targetFrames) {
      if (this.recovering) this.underrunFrames += length;
      return true;
    }
    this.started = true;
    this.recovering = false;
    this.averageQueue += (this.count - this.phase - this.averageQueue) * Math.min(1, length / 24000);
    this.stableFrames += length;
    if (this.stableFrames >= 96000) {
      // A slow, bounded clock correction uses one fractional position for both channels.
      const error = this.averageQueue - (this.targetFrames - 240);
      this.drift = Math.max(-0.001, Math.min(0.001, this.drift + error * length / 48000 * 0.0000001));
      this.rate = 1 + Math.max(-0.001, Math.min(0.001, this.drift + error * 0.000001));
    }
    let frame = 0;
    for (; frame < length; frame++) {
      const integer = this.phase === 0 && this.rate === 1;
      if (!this.count || (!integer && this.count <= NATIVE_AUDIO_LOOKAHEAD)) break;
      let left = this.left[this.read], right = this.right[this.read];
      if (!integer) {
        const position = this.phase * NATIVE_AUDIO_PHASES;
        const table = Math.floor(position) * NATIVE_AUDIO_TAPS;
        const blend = position - Math.floor(position);
        left = right = 0;
        for (let tap = 0; tap < NATIVE_AUDIO_TAPS; tap++) {
          const relative = tap - NATIVE_AUDIO_HISTORY;
          const coefficient = NATIVE_AUDIO_KERNEL[table + tap] + (NATIVE_AUDIO_KERNEL[table + NATIVE_AUDIO_TAPS + tap] - NATIVE_AUDIO_KERNEL[table + tap]) * blend;
          if (relative < 0) {
            if (-relative > this.historyCount) continue;
            const index = (this.historyWrite + NATIVE_AUDIO_HISTORY + relative) % NATIVE_AUDIO_HISTORY;
            left += this.historyLeft[index] * coefficient;
            right += this.historyRight[index] * coefficient;
          } else {
            const index = (this.read + relative) % this.capacity;
            left += this.left[index] * coefficient;
            right += this.right[index] * coefficient;
          }
        }
      }
      if (output[0]) output[0][frame] = left;
      if (output[1]) output[1][frame] = right;
      this.phase += this.rate;
      const advance = Math.min(this.count, Math.floor(this.phase));
      for (let step = 0; step < advance; step++) {
        this.historyLeft[this.historyWrite] = this.left[this.read];
        this.historyRight[this.historyWrite] = this.right[this.read];
        this.historyWrite = (this.historyWrite + 1) % NATIVE_AUDIO_HISTORY;
        this.historyCount = Math.min(NATIVE_AUDIO_HISTORY, this.historyCount + 1);
        this.read = (this.read + 1) % this.capacity;
      }
      this.count -= advance;
      this.phase -= advance;
    }
    if (frame < length) {
      this.underruns++;
      this.underrunFrames += length - frame;
      this.started = false;
      this.recovering = true;
      this.targetFrames = Math.min(this.maxFrames, this.targetFrames + 480);
      this.stableFrames = 0;
      this.averageQueue = this.targetFrames - 240;
      this.rate = 1;
      this.drift = 0;
    } else if (this.targetFrames > 2400 && this.stableFrames >= 480000) {
      this.targetFrames -= 480;
      this.stableFrames = 0;
    }
    return true;
  }
}

registerProcessor("localdrop-native-audio", LocalDropNativeAudioProcessor);
