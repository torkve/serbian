import { h } from '../lib/dom.js';

// Band labels mirror the server-side calibration (1 = B2-low, 6 = C1+ литерарно).
// Kept in sync by hand — there are only six entries and they change rarely.
const BAND_LABELS = {
  1: 'B2-low',
  2: 'B2-high',
  3: 'C1-low',
  4: 'C1-mid',
  5: 'C1-high',
  6: 'C1+ литерарно',
};

function bandOption(level, selected) {
  return h('option',
    { value: String(level), selected: level === selected ? 'selected' : null },
    `${level} — ${BAND_LABELS[level]}`);
}

function bandSelect(name, value, onChange) {
  const opts = [];
  for (let i = 1; i <= 6; i++) opts.push(bandOption(i, value));
  return h('select', {
    name,
    onChange: (e) => onChange(parseInt(e.target.value, 10)),
  }, ...opts);
}

export function settings({ prefs, draft, saving, error, onChangeMin, onChangeMax, onSave, onBack }) {
  const invalid = draft.difficulty_min > draft.difficulty_max;
  const dirty = draft.difficulty_min !== prefs.difficulty_min
             || draft.difficulty_max !== prefs.difficulty_max;

  return h('section', { class: 'view view-settings' },
    h('h1', null, 'Подешавања'),
    h('div', { class: 'field' },
      h('label', null, 'Опсег тежине'),
      h('div', { class: 'range-row' },
        bandSelect('difficulty_min', draft.difficulty_min, onChangeMin),
        h('span', { class: 'range-sep' }, '—'),
        bandSelect('difficulty_max', draft.difficulty_max, onChangeMax),
      ),
      h('p', { class: 'hint' },
        'Сесије ће садржати само задатке у изабраном опсегу. Подразумевано 3—6.'),
      invalid
        ? h('p', { class: 'error' }, 'Доња граница не сме бити већа од горње.')
        : null,
      error ? h('p', { class: 'error' }, error) : null,
    ),
    h('div', { class: 'actions' },
      h('button',
        { class: 'btn-primary', disabled: (!dirty || invalid || saving) ? 'disabled' : null, onClick: onSave },
        saving ? 'Чувам…' : 'Сачувај'),
      h('button', { class: 'btn-secondary', onClick: onBack }, '← Назад'),
    ),
  );
}
