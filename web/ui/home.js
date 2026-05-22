import { h } from '../lib/dom.js';

export function home({ onStart, onOpenSettings, pushState, onEnablePush, onTestPush, onDisablePush }) {
  let pushRow = null;
  if (pushState && pushState.supported) {
    if (pushState.subscribed) {
      pushRow = h('div', { class: 'push-row' },
        h('p', { class: 'lead' }, 'Подсетници су укључени.'),
        h('div', { class: 'actions' },
          h('button', { class: 'btn-secondary', onClick: onTestPush }, 'Тест'),
          h('button', { class: 'btn-secondary', onClick: onDisablePush }, 'Искључи'),
        ),
      );
    } else {
      pushRow = h('div', { class: 'push-row' },
        h('button', { class: 'btn-secondary', onClick: onEnablePush }, 'Укључи подсетнике'),
      );
    }
  }

  return h('section', { class: 'view view-home' },
    h('h1', null, 'Тренер за српски'),
    h('p', { class: 'lead' }, 'Кратка вежба од око два минута: граматика, превод и говор.'),
    h('button', { class: 'btn-primary', onClick: onStart }, 'Започни вежбу'),
    pushRow,
    h('div', { class: 'home-footer' },
      h('button', { class: 'btn-link', onClick: onOpenSettings }, 'Подешавања'),
    ),
  );
}
