import { h } from '../lib/dom.js';
import { bandShort } from '../lib/bands.js';

const labels = {
  cloze: 'Попуни празнину',
  conjugation: 'Граматички облик',
  case: 'Изабери облик',
  aspect: 'Изабери вид',
  tr_ru_sr: 'Превод на српски',
  tr_sr_ru: 'Превод на руски',
  vocab: 'Лексика',
  speak: 'Изговор',
};

export function session(ctx) {
  const { task, index, total, checked, lastResult, onCheck, onNext, onFinish, onAnswer } = ctx;

  const diffPill = (typeof task.difficulty === 'number')
    ? h('span', { class: 'diff-pill' }, bandShort(task.difficulty))
    : null;

  const progress = h('div', { class: 'progress' },
    h('span', null, `${index + 1} / ${total}`),
    h('span', { class: 'kind-pill' }, labels[task.kind] || task.kind),
    diffPill,
  );

  // For `speak`, renderSpeak() already shows the sentence as its .target —
  // skip the generic prompt element to avoid the duplicate.
  const promptEl = task.kind === 'speak'
    ? null
    : h('div', { class: 'prompt' }, task.prompt);

  const hint = task.payload && task.payload.hint
    ? h('div', { class: 'hint' }, `Подсетник: ${task.payload.hint}`)
    : null;

  let inputEl, ruGloss = null;
  switch (task.kind) {
    case 'case':
    case 'aspect':
      inputEl = renderChoices(task, ctx);
      if (task.kind === 'aspect' && task.payload && task.payload.ru) {
        ruGloss = h('div', { class: 'gloss' }, `На руском: ${task.payload.ru}`);
      }
      break;
    case 'tr_ru_sr':
    case 'tr_sr_ru':
      inputEl = renderTextarea(task, ctx);
      break;
    case 'speak':
      inputEl = renderSpeak(task, ctx);
      break;
    case 'vocab':
      inputEl = renderText(task, ctx, 'Превод на руски');
      break;
    default:
      inputEl = renderText(task, ctx, 'Одговор (ћирилица)');
  }

  let actions;
  if (checked) {
    actions = h('div', { class: 'actions' },
      h('button', { class: 'btn-primary', onClick: index + 1 < total ? onNext : onFinish },
        index + 1 < total ? 'Даље' : 'Заврши'));
  } else if (task.kind === 'speak') {
    // Speak tasks self-submit on stop-record; no Provide button.
    actions = null;
  } else {
    actions = h('div', { class: 'actions' },
      h('button', { class: 'btn-primary', onClick: onCheck, id: 'check-btn' }, 'Провери'));
  }

  const verdict = checked && lastResult ? renderVerdict(lastResult, ctx.answer, task) : null;

  return h('section', { class: 'view view-session' },
    progress, promptEl, ruGloss, hint, inputEl, verdict, actions);
}

function renderText(task, ctx, placeholder) {
  return h('input', {
    type: 'text',
    autocomplete: 'off',
    autocapitalize: 'none',
    spellcheck: 'false',
    lang: task.kind === 'tr_sr_ru' || task.kind === 'vocab' ? 'ru' : 'sr-Cyrl-RS',
    inputmode: 'text',
    placeholder,
    disabled: ctx.checked,
    value: ctx.answer || '',
    onInput: (e) => ctx.onAnswer(e.target.value),
    onKeydown: (e) => { if (e.key === 'Enter' && !ctx.checked) ctx.onCheck(); },
  });
}

function renderTextarea(task, ctx) {
  return h('textarea', {
    rows: '3',
    autocapitalize: 'sentences',
    spellcheck: 'false',
    lang: task.kind === 'tr_sr_ru' ? 'ru' : 'sr-Cyrl-RS',
    placeholder: task.kind === 'tr_sr_ru' ? 'Превод на руски…' : 'Превод на српски (ћирилица)…',
    disabled: ctx.checked,
    value: ctx.answer || '',
    onInput: (e) => ctx.onAnswer(e.target.value),
  });
}

function renderChoices(task, ctx) {
  const opts = (task.payload && task.payload.options) || [];
  return h('div', { class: 'choices' },
    ...opts.map((opt) =>
      h('button', {
        class: 'choice' + (ctx.answer === opt ? ' selected' : ''),
        disabled: ctx.checked,
        onClick: () => { ctx.onAnswer(opt); ctx.onCheck(); },
      }, opt),
    ),
  );
}

function renderSpeak(task, ctx) {
  const target = (task.payload && task.payload.target_sr) || task.prompt;
  const recording = ctx.recording;
  const transcribing = ctx.transcribing;

  // Primary button cycles: Снимај -> Заустави -> Слушам… (disabled).
  let label, cls, onClick, disabled = false;
  if (transcribing) {
    label = 'Слушам…';
    cls = 'btn-primary';
    onClick = null;
    disabled = true;
  } else if (recording) {
    label = 'Заустави';
    cls = 'btn-recording';
    onClick = ctx.onStopRecord;
  } else {
    label = 'Снимај';
    cls = 'btn-primary';
    onClick = ctx.onStartRecord;
  }

  // While transcribing we offer "Откажи" so the user can bail on a slow
  // upload. "Прескочи" is available in every pre-check state.
  const cancelBtn = transcribing
    ? h('button', { class: 'btn-secondary', onClick: ctx.onCancelTranscribe }, 'Откажи')
    : null;
  const skipBtn = !ctx.checked
    ? h('button', {
        class: 'btn-secondary',
        onClick: ctx.onSkip,
        disabled: ctx.skipping,
      }, 'Прескочи')
    : null;

  return h('div', { class: 'speak' },
    h('p', { class: 'target' }, target),
    h('div', { class: 'actions' },
      h('button', { class: cls, disabled: disabled || ctx.checked, onClick }, label),
      cancelBtn,
      skipBtn,
    ),
    recording ? h('p', { class: 'hint blink' }, 'Снимам…') : null,
    transcribing ? h('p', { class: 'hint' }, 'Шаљем у препознавање…') : null,
  );
}

// highlightSubstrings renders `text` as a mix of plain text nodes and
// <mark class={cls}> elements wrapping every case-insensitive occurrence of
// any string in `list`. Empty list or no matches => returns a single text
// node. Greedy left-to-right; overlapping needles favour the first match.
function highlightSubstrings(text, list, cls) {
  if (!text) return [document.createTextNode('')];
  if (!list || !list.length) return [document.createTextNode(String(text))];
  const haystack = String(text);
  const lowerHay = haystack.toLowerCase();
  // Collect [start, end) ranges in source-string offsets.
  const ranges = [];
  for (const needle of list) {
    if (!needle) continue;
    const n = String(needle).toLowerCase();
    if (!n) continue;
    let from = 0;
    while (true) {
      const i = lowerHay.indexOf(n, from);
      if (i < 0) break;
      ranges.push([i, i + n.length]);
      from = i + n.length;
    }
  }
  if (!ranges.length) return [document.createTextNode(haystack)];
  // Sort + merge overlapping ranges.
  ranges.sort((a, b) => a[0] - b[0]);
  const merged = [ranges[0]];
  for (let i = 1; i < ranges.length; i++) {
    const last = merged[merged.length - 1];
    if (ranges[i][0] <= last[1]) {
      last[1] = Math.max(last[1], ranges[i][1]);
    } else {
      merged.push(ranges[i]);
    }
  }
  // Emit alternating plain / marked nodes.
  const out = [];
  let cursor = 0;
  for (const [s, e] of merged) {
    if (s > cursor) out.push(document.createTextNode(haystack.slice(cursor, s)));
    out.push(h('mark', { class: cls }, haystack.slice(s, e)));
    cursor = e;
  }
  if (cursor < haystack.length) out.push(document.createTextNode(haystack.slice(cursor)));
  return out;
}

function renderVerdict(res, userAnswer, task) {
  const cls = 'verdict ' + (res.correct ? 'ok' : 'bad');
  const head = res.correct
    ? (res.grade === 5 ? 'Тачно!' : 'Добро.')
    : (res.grade === 0 ? 'Нетачно.' : 'Скоро.');
  const missing = Array.isArray(res.missing_critical) ? res.missing_critical : [];
  const hitForbidden = Array.isArray(res.hit_forbidden) ? res.hit_forbidden : [];
  // For speak tasks the user's answer is whatever Whisper transcribed; the
  // server returns that as feedback ("Транскрипт: …"). For everything else
  // we already hold the typed/selected answer in session state.
  let mine = null;
  if (task && task.kind === 'speak') {
    if (res.feedback) {
      const transcript = res.feedback.replace(/^Транскрипт:\s*/, '');
      mine = h('div', { class: 'your-answer' },
        h('strong', null, 'Ваш изговор: '),
        ...highlightSubstrings(transcript, hitForbidden, 'bad'));
    }
  } else if (userAnswer && String(userAnswer).trim() !== '') {
    mine = h('div', { class: 'your-answer' },
      h('strong', null, 'Ваш одговор: '),
      ...highlightSubstrings(userAnswer, hitForbidden, 'bad'));
  }
  const expected = res.expected && res.expected.length
    ? h('div', null,
        h('strong', null, 'Очекивано: '),
        ...highlightSubstrings(res.expected.join(' / '), missing, 'ok'))
    : null;
  const missingRow = missing.length
    ? h('div', { class: 'missing-critical' },
        h('strong', null, 'Недостаје обавезни део: '),
        missing.map((s) => `«${s}»`).join(' / '))
    : null;
  const forbiddenRow = hitForbidden.length
    ? h('div', { class: 'hit-forbidden' },
        h('strong', null, 'Не сме садржати: '),
        hitForbidden.map((s) => `«${s}»`).join(' / '))
    : null;
  const rationale = res.rationale
    ? h('div', { class: 'rationale' }, h('strong', null, 'Објашњење: '), res.rationale)
    : null;
  const sim = typeof res.similarity === 'number' && res.graded_by !== 'exact' && res.graded_by !== 'critical' && res.graded_by !== 'forbidden'
    ? h('div', { class: 'sim' }, `Сличност: ${(res.similarity * 100).toFixed(0)}%`)
    : null;
  return h('div', { class: cls },
    h('div', { class: 'verdict-head' }, head),
    mine, expected, missingRow, forbiddenRow, sim, rationale);
}
