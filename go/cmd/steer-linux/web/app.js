/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 入口：路由（hash 驱动）+ 启动 + 未保存守卫。 */
'use strict';
(function () {
  const S = window.S;

  function render(name) {
    const view = S.views[name] || S.views.overview;
    const root = document.querySelector('#view');
    const routeToken = S.ui.beginRoute(root);
    document.querySelectorAll('.nav-item').forEach((el) => el.classList.toggle('is-active', el.dataset.view === name));
    Promise.resolve(view.render(root)).catch((e) => {
      if (!S.ui.isCurrentRoute(root, routeToken)) return;
      root.append(S.h('div', { class: 'card card--edge edge--err' }, [
        S.h('strong', {}, '渲染失败'),
        S.h('pre', { class: 'mono' }, e.stack || e.message)
      ]));
    });
    root.scrollTop = 0;
  }

  function canOpen(name) {
    if (name === 'advanced' || S.store.draftValid !== false) return true;
    S.ui.toast(`当前 JSON 配置格式有误，不能打开此页面：${S.store.draftError}`, 'err');
    return false;
  }

  function renderRequested(name) {
    if (canOpen(name)) {
      render(name);
      return true;
    }
    history.replaceState(null, '', '#/advanced');
    render('advanced');
    return false;
  }

  function router(name) {
    if (!canOpen(name)) return false;
    if (location.hash === `#/${name}`) render(name);
    else location.hash = `#/${name}`;
    return true;
  }
  S.router = router;

  function currentView() {
    return (location.hash.match(/^#\/([a-z]+)/) || [])[1] || 'overview';
  }
  S.renderCurrent = () => renderRequested(currentView());

  function boot() {
    S.ui.renderShell(router);
    S.ui.renderStatusStrip();
    S.store.subscribe(S.ui.renderStatusStrip);

    if (!location.hash) history.replaceState(null, '', '#/overview');
    renderRequested(currentView());
    window.addEventListener('hashchange', () => renderRequested(currentView()));

    let refreshInFlight = false;
    const refreshVisibleState = async () => {
      if (refreshInFlight || document.visibilityState === 'hidden') return;
      refreshInFlight = true;
      try { await S.store.refreshServerState(); } catch (_) { /* explicit Refresh reports errors */ }
      finally { refreshInFlight = false; }
    };
    window.setInterval?.(refreshVisibleState, 30_000);
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') refreshVisibleState();
    });

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
