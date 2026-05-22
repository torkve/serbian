// One source of truth for difficulty band labels. Long form for the
// settings picker, short form for the in-session badge.

export const BAND_LABELS = {
  1: 'B2-low',
  2: 'B2-high',
  3: 'C1-low',
  4: 'C1-mid',
  5: 'C1-high',
  6: 'C1+ литерарно',
  7: 'C2 академски/књижевни',
  8: 'C2+ мајсторски',
};

const SHORT = {
  1: 'B2-low', 2: 'B2-high',
  3: 'C1-low', 4: 'C1-mid', 5: 'C1-high', 6: 'C1+',
  7: 'C2',     8: 'C2+',
};

export function bandShort(level) {
  return SHORT[level] || `d${level}`;
}
