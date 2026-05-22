import { h } from '../lib/dom.js';

export function results({ summary, onAgain }) {
  const pct = summary.attempts > 0
    ? Math.round((summary.correct / summary.attempts) * 100)
    : 0;
  return h('section', { class: 'view view-results' },
    h('h1', null, 'Готово'),
    h('p', { class: 'big-number' }, `${summary.correct} / ${summary.attempts}`),
    h('p', { class: 'lead' }, `Тачно ${pct}% задатака за ${summary.duration_sec || 0} с.`),
    h('button', { class: 'btn-primary', onClick: onAgain }, 'Још једна вежба'),
  );
}
