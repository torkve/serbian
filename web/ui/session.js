import { h } from '../lib/dom.js';

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

  const progress = h('div', { class: 'progress' },
    h('span', null, `${index + 1} / ${total}`),
    h('span', { class: 'kind-pill' }, labels[task.kind] || task.kind),
  );

  const promptEl = h('div', { class: 'prompt' }, task.prompt);

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

function renderVerdict(res, userAnswer, task) {
  const cls = 'verdict ' + (res.correct ? 'ok' : 'bad');
  const head = res.correct
    ? (res.grade === 5 ? 'Тачно!' : 'Добро.')
    : (res.grade === 0 ? 'Нетачно.' : 'Скоро.');
  // For speak tasks the user's answer is whatever Whisper transcribed; the
  // server returns that as feedback ("Транскрипт: …"). For everything else
  // we already hold the typed/selected answer in session state.
  let mine = null;
  if (task && task.kind === 'speak') {
    if (res.feedback) {
      mine = h('div', { class: 'your-answer' }, h('strong', null, 'Ваш изговор: '), res.feedback.replace(/^Транскрипт:\s*/, ''));
    }
  } else if (userAnswer && String(userAnswer).trim() !== '') {
    mine = h('div', { class: 'your-answer' }, h('strong', null, 'Ваш одговор: '), userAnswer);
  }
  const expected = res.expected && res.expected.length
    ? h('div', null, h('strong', null, 'Очекивано: '), res.expected.join(' / '))
    : null;
  const rationale = res.rationale
    ? h('div', { class: 'rationale' }, h('strong', null, 'Објашњење: '), res.rationale)
    : null;
  const sim = typeof res.similarity === 'number' && res.graded_by !== 'exact'
    ? h('div', { class: 'sim' }, `Сличност: ${(res.similarity * 100).toFixed(0)}%`)
    : null;
  return h('div', { class: cls }, h('div', { class: 'verdict-head' }, head), mine, expected, sim, rationale);
}
