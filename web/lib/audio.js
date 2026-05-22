// Thin MediaRecorder wrapper for the speaking practice tasks.
// Records to webm/opus when supported, falls back to whatever the browser
// allows (Safari typically gives audio/mp4). Whisper-server handles both.

// Hard ceiling on recording length. Speak prompts are short sentences, so
// 20 s is comfortable even for slow delivery; without a cap a stuck recorder
// or a forgotten tap leaves the UI hanging.
export const MAX_RECORD_MS = 20000;

// makeRecorder({ onAutoStop }) — the optional onAutoStop callback fires when
// the recorder hits MAX_RECORD_MS and stops itself. The caller can use it to
// transition UI state (from "recording" to "transcribing").
export function makeRecorder(opts = {}) {
  let recorder = null;
  let chunks = [];
  let stream = null;
  let startedAt = 0;
  let maxTimer = null;
  let autoStopped = false;

  const candidates = [
    'audio/webm;codecs=opus',
    'audio/webm',
    'audio/mp4',
    'audio/ogg;codecs=opus',
    'audio/ogg',
  ];

  function clearMaxTimer() {
    if (maxTimer != null) {
      clearTimeout(maxTimer);
      maxTimer = null;
    }
  }

  return {
    maxRecordMs: MAX_RECORD_MS,

    async start() {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      chunks = [];
      autoStopped = false;
      const mime = candidates.find(
        (t) => typeof MediaRecorder !== 'undefined' && MediaRecorder.isTypeSupported(t),
      );
      recorder = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined);
      recorder.addEventListener('dataavailable', (e) => {
        if (e.data && e.data.size > 0) chunks.push(e.data);
      });
      startedAt = Date.now();
      recorder.start();

      // Auto-stop safety net. We don't call stop() here directly — we trigger
      // the MediaRecorder's stop event, which the caller's stop()-promise
      // will pick up. The onAutoStop hook lets the UI react before the
      // promise resolves.
      maxTimer = setTimeout(() => {
        autoStopped = true;
        maxTimer = null;
        if (typeof opts.onAutoStop === 'function') {
          try { opts.onAutoStop(); } catch (_) {}
        }
        try {
          if (recorder && recorder.state !== 'inactive') recorder.stop();
        } catch (_) {}
      }, MAX_RECORD_MS);
    },

    // Returns { blob, durationMs, mimeType, autoStopped } once the underlying
    // MediaRecorder emits 'stop'. Safe to call after auto-stop fired.
    stop() {
      return new Promise((resolve, reject) => {
        if (!recorder) {
          clearMaxTimer();
          reject(new Error('not recording'));
          return;
        }
        const finish = () => {
          clearMaxTimer();
          const mimeType = recorder.mimeType || 'audio/webm';
          const blob = new Blob(chunks, { type: mimeType });
          if (stream) {
            stream.getTracks().forEach((t) => t.stop());
            stream = null;
          }
          const result = {
            blob,
            durationMs: Date.now() - startedAt,
            mimeType,
            autoStopped,
          };
          recorder = null;
          chunks = [];
          resolve(result);
        };
        if (recorder.state === 'inactive') {
          // Already stopped (e.g. auto-stop fired before user tapped). Resolve
          // immediately with whatever chunks we've collected.
          finish();
          return;
        }
        recorder.addEventListener('stop', finish, { once: true });
        recorder.addEventListener('error', (e) => {
          clearMaxTimer();
          reject(e);
        }, { once: true });
        try {
          recorder.stop();
        } catch (e) {
          clearMaxTimer();
          reject(e);
        }
      });
    },

    // Discard recording without uploading. Used by the Skip button while
    // recording is in progress.
    cancel() {
      clearMaxTimer();
      try {
        if (recorder && recorder.state !== 'inactive') recorder.stop();
      } catch (_) {}
      if (stream) {
        stream.getTracks().forEach((t) => t.stop());
        stream = null;
      }
      recorder = null;
      chunks = [];
    },
  };
}
