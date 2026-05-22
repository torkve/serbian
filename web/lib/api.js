// Whisper transcription is normally <5s on a local whisper-server, but
// cold-starts and big audio blobs can stretch that out. We cap the client
// wait so the UI can offer "Откажи / Прескочи" instead of hanging forever.
// The server's own STT timeout is 2 minutes, so this strictly tightens it.
const SPEAK_TIMEOUT_MS = 45000;

async function jsonFetch(url, opts = {}) {
  const headers = Object.assign({ 'Content-Type': 'application/json' }, opts.headers || {});
  const r = await fetch(url, Object.assign({}, opts, { headers }));
  if (!r.ok) {
    const text = await r.text().catch(() => '');
    throw new Error(`HTTP ${r.status}: ${text || r.statusText}`);
  }
  return r.json();
}

// Tags an AbortError thrown by fetch() so callers can tell user-cancel from
// timeout from network error.
function tagAbortError(err, reason) {
  if (err && err.name === 'AbortError') {
    err.reason = reason;
  }
  return err;
}

// All fetch URLs are relative. They resolve against the document URL, so the
// app works whether mounted at the origin root or at any subpath behind a
// reverse proxy. The trailing slash on the served document (e.g. /serbian/)
// matters — without it, relative URLs would resolve against /. nginx config
// should redirect /serbian -> /serbian/ to be safe.
export const api = {
  startSession: () => jsonFetch('api/session/start', { method: 'POST' }),
  attempt: (sid, body) => jsonFetch(`api/session/${sid}/attempt`, {
    method: 'POST',
    body: JSON.stringify(body),
  }),
  // speakAttempt uploads recorded audio for STT + grading.
  //   opts.signal — optional external AbortSignal (e.g. user-pressed cancel).
  //                  The internal timeout fires independently; whichever
  //                  aborts first wins. The returned promise rejects with an
  //                  AbortError whose `.reason` is 'user' or 'timeout'.
  speakAttempt: async (sid, taskId, blob, durationMs, filename, opts = {}) => {
    const form = new FormData();
    form.append('task_id', String(taskId));
    form.append('duration_ms', String(durationMs));
    form.append('audio', blob, filename || 'audio.webm');

    const ctl = new AbortController();
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      ctl.abort();
    }, SPEAK_TIMEOUT_MS);
    let userCancelled = false;
    const onExternalAbort = () => {
      userCancelled = true;
      ctl.abort();
    };
    if (opts.signal) {
      if (opts.signal.aborted) onExternalAbort();
      else opts.signal.addEventListener('abort', onExternalAbort, { once: true });
    }

    try {
      const r = await fetch(`api/session/${sid}/speak`, {
        method: 'POST',
        body: form,
        signal: ctl.signal,
      });
      if (!r.ok) {
        const text = await r.text().catch(() => '');
        throw new Error(`HTTP ${r.status}: ${text || r.statusText}`);
      }
      return r.json();
    } catch (e) {
      if (e && e.name === 'AbortError') {
        throw tagAbortError(e, userCancelled ? 'user' : (timedOut ? 'timeout' : 'unknown'));
      }
      throw e;
    } finally {
      clearTimeout(timer);
      if (opts.signal) opts.signal.removeEventListener('abort', onExternalAbort);
    }
  },
  skipTask: (sid, taskId, durationMs) => jsonFetch(`api/session/${sid}/skip`, {
    method: 'POST',
    body: JSON.stringify({ task_id: taskId, duration_ms: durationMs }),
  }),
  endSession: (sid) => jsonFetch(`api/session/${sid}/end`, { method: 'POST' }),
  getMe: () => jsonFetch('api/me'),
  updatePrefs: (body) => jsonFetch('api/me/prefs', {
    method: 'PATCH',
    body: JSON.stringify(body),
  }),
};
