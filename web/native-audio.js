(() => {
  "use strict";

  const SAMPLE_RATE = 48000;
  const MAX_PCM_BYTES = 32768;
  const MAX_QUEUED_FRAMES = SAMPLE_RATE * 0.06;
  const nativeError = (code, message) => Object.assign(new Error(typeof message === "string" && message.length <= 512 ? message : "Native audio capture failed."), { name: "NativeAudioError", code });
  const supported = () => window.isSecureContext && typeof window.AudioContext === "function" && typeof window.AudioWorkletNode === "function" && typeof window.WebSocket === "function";

  function open({ connectionId, target, signal } = {}) {
    return new Promise((resolve, reject) => {
      let context, destination, processor, socket, stream;
      let closed = false, settled = false, ready = false;
      let sequence = 0, inFlight = 0, inFlightFrames = 0, pendingFrames = 0, discardedFrames = 0;
      const pending = [];
      let playbackStats = null;
      let timer;
      const close = () => {
        if (closed) return;
        closed = true;
        clearTimeout(timer);
        signal?.removeEventListener("abort", abort);
        pending.length = 0;
        pendingFrames = inFlightFrames = inFlight = 0;
        if (socket) {
          socket.onmessage = socket.onerror = socket.onclose = null;
          try { socket.close(); } catch { /* The socket may still be connecting. */ }
        }
        if (processor) {
          processor.onprocessorerror = null;
          processor.port.onmessage = null;
          try { processor.port.postMessage({ type: "close" }); } catch { /* The worklet may already be closed. */ }
          processor.port.close();
          processor.disconnect();
        }
        stream?.getTracks().forEach((track) => track.stop());
        if (context) {
          context.onstatechange = null;
          try { Promise.resolve(context.close()).catch(() => {}); } catch { /* The context may already be closed. */ }
        }
      };
      const fail = (error) => {
        if (closed) return;
        const active = settled;
        close();
        if (!active) { settled = true; reject(error); }
        else {
          const event = new Event("native-audio-error");
          event.error = error;
          stream.dispatchEvent(event);
        }
      };
      const abort = () => fail(new DOMException("Capture cancelled.", "AbortError"));
      const postPCM = (buffer) => {
        inFlight = ++sequence;
        inFlightFrames = buffer.byteLength / 4;
        processor.port.postMessage({ type: "pcm", id: inFlight, buffer }, [buffer]);
      };
      const stats = () => ({ mainQueueMs: (pendingFrames + inFlightFrames) * 1000 / SAMPLE_RATE, discardedFrames, ...playbackStats });
      const queuePCM = (buffer) => {
        pending.push(buffer);
        pendingFrames += buffer.byteLength / 4;
        // Only discard the oldest audio when delivery falls more than 60ms behind.
        let excess = Math.max(0, pendingFrames + inFlightFrames - MAX_QUEUED_FRAMES);
        discardedFrames += excess;
        while (excess) {
          const frames = pending[0].byteLength / 4;
          const drop = Math.min(excess, frames);
          if (drop === frames) pending.shift();
          else pending[0] = pending[0].slice(drop * 4);
          pendingFrames -= drop;
          excess -= drop;
        }
        if (!inFlight && pending.length) {
          const next = pending.shift();
          pendingFrames -= next.byteLength / 4;
          postPCM(next);
        }
      };
      if (signal?.aborted) { abort(); return; }
      if (!supported()) { fail(nativeError("unsupported")); return; }
      if (typeof connectionId !== "string" || !connectionId || typeof target !== "string" || !target) { fail(nativeError("unavailable")); return; }
      signal?.addEventListener("abort", abort, { once: true });
      timer = setTimeout(() => fail(nativeError("timeout")), 10000);

      // Construction and resume stay inside the caller's sharing gesture.
      try {
        context = new window.AudioContext({ sampleRate: SAMPLE_RATE, latencyHint: "interactive" });
        if (context.sampleRate !== SAMPLE_RATE || !context.audioWorklet?.addModule) { fail(nativeError("format")); return; }
        const resumed = context.resume();
        const loaded = context.audioWorklet.addModule("/native-audio-worklet.js");
        Promise.all([resumed, loaded]).then(() => {
          if (closed) return;
          if (context.state !== "running") { fail(nativeError("unsupported")); return; }
          destination = context.createMediaStreamDestination();
          destination.channelCount = 2;
          destination.channelCountMode = "explicit";
          stream = destination.stream;
          const tracks = stream.getAudioTracks();
          const settings = tracks[0]?.getSettings?.() || {};
          if (tracks.length !== 1 || tracks[0].readyState !== "live" || (settings.sampleRate !== undefined && settings.sampleRate !== SAMPLE_RATE) || (settings.channelCount !== undefined && settings.channelCount !== 2)) { fail(nativeError("format")); return; }
          processor = new window.AudioWorkletNode(context, "localdrop-native-audio", { numberOfInputs: 0, numberOfOutputs: 1, outputChannelCount: [2], channelCount: 2, channelCountMode: "explicit" });
          processor.onprocessorerror = () => fail(nativeError("interrupted"));
          processor.port.onmessage = ({ data }) => {
            if (closed) return;
            if (data?.type === "stats" || (data?.type === "accepted" && data.id === inFlight)) {
              if (data.stats) playbackStats = data.stats;
            }
            if (data?.type !== "accepted" || data.id !== inFlight) return;
            inFlight = 0;
            inFlightFrames = 0;
            if (pending.length) {
              const buffer = pending.shift();
              pendingFrames -= buffer.byteLength / 4;
              postPCM(buffer);
            }
          };
          // This graph has no connection to the laptop's audible destination.
          processor.connect(destination);
          context.onstatechange = () => { if (context.state === "closed") fail(nativeError("interrupted")); };
          const url = new URL("/api/system-audio", window.location.href);
          url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
          url.searchParams.set("connectionId", connectionId);
          url.searchParams.set("target", target);
          socket = new window.WebSocket(url.href);
          socket.binaryType = "arraybuffer";
          socket.onerror = socket.onclose = () => fail(nativeError(ready ? "interrupted" : "unavailable"));
          socket.onmessage = ({ data }) => {
            if (closed) return;
            try {
              if (typeof data === "string") {
                if (data.length > 1024) throw nativeError("format");
                const metadata = JSON.parse(data);
                if (metadata?.type === "error") { fail(nativeError(ready ? "interrupted" : "unavailable", metadata.message)); return; }
                if (ready || !metadata || Array.isArray(metadata) || Object.keys(metadata).length !== 4 || metadata.type !== "ready" || metadata.sampleRate !== SAMPLE_RATE || metadata.channels !== 2 || metadata.sampleSize !== 16) throw nativeError("format");
                ready = true;
                settled = true;
                clearTimeout(timer);
                resolve({ stream, close, stats });
                return;
              }
              if (!ready || !(data instanceof ArrayBuffer) || !data.byteLength || data.byteLength > MAX_PCM_BYTES || data.byteLength % 4) throw nativeError("format");
              queuePCM(data);
            } catch (error) { fail(error?.name === "NativeAudioError" ? error : nativeError("format")); }
          };
        }).catch(() => fail(nativeError("unavailable")));
      } catch { fail(nativeError("unsupported")); }
    });
  }

  window.LocalDropNativeAudio = { open, supported };
})();
