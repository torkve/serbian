// Imports use relative module specifiers so the app runs at any subpath
// when proxied behind nginx (e.g. https://example.com/serbian/).
import { mount } from './lib/dom.js';
import { api } from './lib/api.js';
import { makeRecorder } from './lib/audio.js';
import * as push from './lib/push.js';
import { home } from './ui/home.js';
import { session } from './ui/session.js';
import { results } from './ui/results.js';
import { settings } from './ui/settings.js';

const root = document.getElementById('app');

const state = {
  view: 'home',
  session: null,   // { id, tasks, index, answer, checked, lastResult, ... }
  summary: null,
  pushState: { supported: false, subscribed: false },
  // Difficulty preference. Server is authoritative; we keep a draft for the
  // settings view so the user can change pickers freely before saving.
  prefs: { difficulty_min: 3, difficulty_max: 6 },
  settings: { draft: null, saving: false, error: null },
};

function render() {
  switch (state.view) {
    case 'home':
      mount(root, home({
        onStart: startSession,
        onOpenSettings: openSettings,
        pushState: state.pushState,
        onEnablePush: enablePush,
        onTestPush: testPush,
        onDisablePush: disablePush,
      }));
      break;
    case 'settings':
      mount(root, settings({
        prefs: state.prefs,
        draft: state.settings.draft,
        saving: state.settings.saving,
        error: state.settings.error,
        onChangeMin: (v) => { state.settings.draft.difficulty_min = v; render(); },
        onChangeMax: (v) => { state.settings.draft.difficulty_max = v; render(); },
        onSave: savePrefs,
        onBack: closeSettings,
      }));
      break;
    case 'session': {
      const s = state.session;
      mount(root, session({
        task: s.tasks[s.index],
        index: s.index,
        total: s.tasks.length,
        answer: s.answer,
        checked: s.checked,
        lastResult: s.lastResult,
        recording: s.recording,
        transcribing: s.transcribing,
        skipping: s.skipping,
        onAnswer: (v) => { s.answer = v; },
        onCheck: () => check(),
        onNext: () => advance(),
        onFinish: () => finish(),
        onStartRecord: () => startRecord(),
        onStopRecord: () => stopRecord(),
        onCancelTranscribe: () => cancelTranscribe(),
        onSkip: () => skip(),
      }));
      // Focus the first input/textarea so user can start typing immediately.
      queueMicrotask(() => {
        const el = root.querySelector('input, textarea');
        if (el && !s.checked) el.focus();
      });
      break;
    }
    case 'results':
      mount(root, results({ summary: state.summary, onAgain: () => { state.view = 'home'; render(); } }));
      break;
  }
}

async function startSession() {
  try {
    const data = await api.startSession();
    if (!data.tasks || data.tasks.length === 0) {
      state.view = 'results';
      state.summary = { attempts: 0, correct: 0, duration_sec: 0 };
      render();
      return;
    }
    state.session = {
      id: data.session_id,
      tasks: data.tasks,
      index: 0,
      answer: '',
      checked: false,
      lastResult: null,
      startedAt: Date.now(),
      taskStartedAt: Date.now(),
    };
    state.view = 'session';
    render();
  } catch (e) {
    alert('Грешка: ' + e.message);
  }
}

async function check() {
  const s = state.session;
  const task = s.tasks[s.index];
  if (task.kind === 'speak') return; // speak uses startRecord/stopRecord
  if (!s.answer || !s.answer.trim()) return;
  try {
    const r = await api.attempt(s.id, {
      task_id: task.id,
      answer: s.answer,
      duration_ms: Date.now() - (s.taskStartedAt || s.startedAt),
    });
    s.lastResult = r;
    s.checked = true;
    render();
  } catch (e) {
    alert('Грешка: ' + e.message);
  }
}

async function startRecord() {
  const s = state.session;
  try {
    // onAutoStop: when the recorder hits its max duration, kick off the same
    // upload path as a user tap on "Заустави". Guard against re-entry.
    s.recorder = makeRecorder({
      onAutoStop: () => {
        if (state.session === s && s.recording && !s.transcribing) {
          stopRecord();
        }
      },
    });
    await s.recorder.start();
    s.recording = true;
    render();
  } catch (e) {
    alert('Грешка микрофона: ' + e.message);
  }
}

async function stopRecord() {
  const s = state.session;
  if (!s.recorder) return;
  if (s.transcribing) return; // ignore double-tap / race with auto-stop
  s.recording = false;
  s.transcribing = true;
  s.abortCtl = new AbortController();
  render();
  try {
    const { blob, durationMs, mimeType } = await s.recorder.stop();
    s.recorder = null;
    const task = s.tasks[s.index];
    const ext = mimeType.startsWith('audio/mp4') ? 'mp4'
      : mimeType.startsWith('audio/ogg') ? 'ogg' : 'webm';
    const r = await api.speakAttempt(s.id, task.id, blob, durationMs,
      `audio.${ext}`, { signal: s.abortCtl.signal });
    s.lastResult = r;
    s.checked = true;
  } catch (e) {
    if (e && e.name === 'AbortError') {
      // Either the user pressed Cancel or the client timeout fired. Don't
      // mark the task as checked — leave the user free to retry or skip.
      const msg = e.reason === 'timeout'
        ? 'Истекло време чекања. Покушај поново или прескочи.'
        : 'Откачено.';
      s.lastResult = null;
      s.transientMsg = msg;
    } else {
      s.lastResult = {
        correct: false, grade: 0, expected: [], similarity: 0,
        rationale: 'Грешка приликом обраде звука: ' + (e && e.message ? e.message : e),
      };
      s.checked = true;
    }
  } finally {
    s.transcribing = false;
    s.abortCtl = null;
    if (s.transientMsg) {
      // Surface the message and then clear it on next render cycle.
      alert(s.transientMsg);
      s.transientMsg = null;
    }
    render();
  }
}

function cancelTranscribe() {
  const s = state.session;
  if (s && s.abortCtl) {
    try { s.abortCtl.abort(); } catch (_) {}
  }
}

async function skip() {
  const s = state.session;
  if (!s || s.skipping) return;
  s.skipping = true;
  // Tear down any in-flight recording or upload first so we don't race with
  // their completion handlers writing into s.lastResult / s.checked.
  if (s.recorder && typeof s.recorder.cancel === 'function') {
    try { s.recorder.cancel(); } catch (_) {}
    s.recorder = null;
  }
  if (s.abortCtl) {
    try { s.abortCtl.abort(); } catch (_) {}
    s.abortCtl = null;
  }
  s.recording = false;
  s.transcribing = false;
  render();
  const task = s.tasks[s.index];
  try {
    await api.skipTask(s.id, task.id, Date.now() - (s.taskStartedAt || s.startedAt));
  } catch (e) {
    // Server-side defer failed — log but still advance locally so the user
    // isn't stuck on the task.
    console.error('skip failed:', e);
  }
  s.skipping = false;
  if (s.index + 1 < s.tasks.length) {
    advance();
  } else {
    await finish();
  }
}

function advance() {
  const s = state.session;
  s.index += 1;
  s.answer = '';
  s.checked = false;
  s.lastResult = null;
  s.recording = false;
  s.transcribing = false;
  s.skipping = false;
  s.recorder = null;
  s.abortCtl = null;
  s.taskStartedAt = Date.now();
  render();
}

async function finish() {
  const s = state.session;
  try {
    const summary = await api.endSession(s.id);
    state.summary = summary;
    state.session = null;
    state.view = 'results';
    render();
  } catch (e) {
    alert('Грешка: ' + e.message);
  }
}

async function openSettings() {
  // Refresh from the server so another device's edits show up. Falls back to
  // whatever we have locally if the request fails (offline / transient).
  try {
    const me = await api.getMe();
    state.prefs = {
      difficulty_min: me.difficulty_min,
      difficulty_max: me.difficulty_max,
    };
  } catch (_) {}
  state.settings = {
    draft: { ...state.prefs },
    saving: false,
    error: null,
  };
  state.view = 'settings';
  render();
}

function closeSettings() {
  state.settings = { draft: null, saving: false, error: null };
  state.view = 'home';
  render();
}

async function savePrefs() {
  const d = state.settings.draft;
  if (!d) return;
  if (d.difficulty_min > d.difficulty_max) return;
  state.settings.saving = true;
  state.settings.error = null;
  render();
  try {
    const updated = await api.updatePrefs({
      difficulty_min: d.difficulty_min,
      difficulty_max: d.difficulty_max,
    });
    state.prefs = {
      difficulty_min: updated.difficulty_min,
      difficulty_max: updated.difficulty_max,
    };
    state.settings = { draft: null, saving: false, error: null };
    state.view = 'home';
    render();
  } catch (e) {
    state.settings.saving = false;
    state.settings.error = e.message || String(e);
    render();
  }
}

async function enablePush() {
  try {
    await push.subscribe();
    state.pushState = await push.pushStatus();
    render();
  } catch (e) {
    alert(e.message || String(e));
  }
}

async function disablePush() {
  try {
    await push.unsubscribe();
    state.pushState = await push.pushStatus();
    render();
  } catch (e) {
    alert(e.message || String(e));
  }
}

async function testPush() {
  try {
    await push.sendTest();
    alert('Тест послат — обавештење ће стићи у наредних неколико секунди.');
  } catch (e) {
    alert(e.message || String(e));
  }
}

render();

(async () => {
  if ('serviceWorker' in navigator) {
    try {
      // Relative path so the SW lands in the same directory the page was
      // loaded from. Its default scope is then exactly this subpath.
      await navigator.serviceWorker.register('./sw.js');
    } catch (_) {}
  }
  state.pushState = await push.pushStatus();
  try {
    const me = await api.getMe();
    state.prefs = {
      difficulty_min: me.difficulty_min,
      difficulty_max: me.difficulty_max,
    };
  } catch (_) {}
  if (state.view === 'home') render();
})();
