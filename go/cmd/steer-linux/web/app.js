/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 入口：路由（hash 驱动）+ 启动 + 未保存守卫。 */
'use strict';
(function () {
  const S = window.S;

  function render(name) {
    const view = S.views[name] || S.views.overview;
    const root = document.querySelector('#view');
    root.replaceChildren();
    document.querySelectorAll('.nav-item').forEach((el) => el.classList.toggle('is-active', el.dataset.view === name));
    Promise.resolve(view.render(root)).catch((e) => {
      root.append(S.h('div', { class: 'card card--edge edge--err' }, [
        S.h('strong', {}, '渲染失败'),
        S.h('pre', { class: 'mono' }, e.stack || e.message)
      ]));
    });
    root.scrollTop = 0;
  }

  function router(name) {
    if (location.hash === `#/${name}`) render(name);
    else location.hash = `#/${name}`;
  }
  S.router = router;

  function currentView() {
    return (location.hash.match(/^#\/([a-z]+)/) || [])[1] || 'overview';
  }

  function boot() {
    S.ui.renderShell(router);
    S.ui.renderStatusStrip();
    S.store.subscribe(S.ui.renderStatusStrip);

    if (!location.hash) history.replaceState(null, '', '#/overview');
    render(currentView());
    window.addEventListener('hashchange', () => render(currentView()));

    window.addEventListener('beforeunload', (e) => {
      if (S.store.dirty) {
        e.preventDefault();
        e.returnValue = '';
      }
    });

  }

  async function start() {
    try {
      await S.store.init();
      boot();
    } catch (e) {
      if (e.code === 'AUTH_REQUIRED') {
        S.auth.show(async () => {
          await S.store.init();
          boot();
        });
        return;
      }
      document.querySelector('#view').append(S.h('div', { class: 'card card--edge edge--err' }, [
        S.h('strong', {}, '初始化失败'),
        S.h('pre', { class: 'mono' }, e.stack || e.message)
      ]));
    }
  }

  document.addEventListener('DOMContentLoaded', start);
})();
