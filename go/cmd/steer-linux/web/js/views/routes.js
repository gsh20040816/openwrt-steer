/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 路由：Direct/Reject 固定语义卡 + single 节点链（node ← detour 链）与环检测。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtLatestProbe } = S;
  const ui = S.ui;
  let routeButtons = [];

  function syncRouteTestButtons() {
    routeButtons.forEach((button) => {
      if (button.classList.contains('spinning')) return;
      button.disabled = !button._eligible || S.store.dirty;
      button.title = !button._eligible ? '已停用路由不能测试' : (S.store.dirty ? '请先保存或放弃工作副本修改' : button._label);
      syncLatest(button);
    });
  }

  function latestResult(routeId, download) {
    const kind = download ? 'download' : 'connect';
    return (S.store.probeResults?.latest_results || []).find((result) =>
      result.scope === 'routes' && result.object_id === routeId && result.kind === kind);
  }

  function syncLatest(button, result = latestResult(button._routeId, button._download)) {
    if (!button._latest) return;
    const latest = fmtLatestProbe(result);
    button._latest.className = `probe-latest ${latest.stale ? 'is-stale' : (latest.ok === false ? 'is-err' : '')}`;
    button._latest.replaceChildren(latest.text);
  }

  function probeAction(button) { return h('div', { class: 'probe-action' }, button, button._latest); }

  function nodeLabel(intent, id) {
    const node = intent.nodes.find((n) => n.id === id);
    return node ? (node.name || id) : id;
  }

  function chain(intent, route) {
    const hops = [];
    let cursor = route.id;
    const seen = new Set();
    while (cursor && !seen.has(cursor)) {
      seen.add(cursor);
      const r = intent.routes.find((x) => x.id === cursor);
      if (!r || r.kind !== 'single') break;
      hops.push(h('span', { class: 'hop' }, nodeLabel(intent, r.node)));
      cursor = r.detour;
    }
    const parts = [];
    hops.forEach((hop, i) => { parts.push(hop); if (i < hops.length - 1) parts.push(h('span', { class: 'arrow' }, '←')); });
    return h('span', { class: 'chain', title: '先连接前置代理' }, parts, hops.length > 1 ? h('span', { class: 'muted' }, '出口') : null);
  }

  function wouldCycle(routeId, detourId, routes) {
    let cursor = detourId;
    const seen = new Set();
    while (cursor) {
      if (cursor === routeId) return true;
      if (seen.has(cursor)) return true;
      seen.add(cursor);
      const r = routes.find((x) => x.id === cursor);
      cursor = r?.detour;
    }
    return false;
  }

  function routeTestButton(label, download, routeId, eligible) {
    const btn = h('button', { class: 'btn btn--sm', onclick: async () => {
      if (!eligible) {
        ui.toast('已停用路由不能测试；请先启用并保存', 'warn');
        return;
      }
      if (S.store.dirty) {
        ui.toast('请先保存或放弃工作副本修改，再测试路由', 'warn');
        return;
      }
      btn.disabled = true; btn.classList.add('spinning'); btn.textContent = '测试中…';
      try {
        const result = await S.api.speedtestRoute(routeId, download);
        S.store.installProbeResult?.(result);
        btn.classList.remove('spinning'); btn.textContent = result.ok ? (result.summary || '成功') : '失败';
        btn.title = result.ok ? '成功' : (result.error_summary || '详细原因请查看诊断日志');
        btn.classList.toggle('is-ok', result.ok); btn.classList.toggle('is-err', !result.ok);
        syncLatest(btn, result);
      } catch (e) {
        btn.classList.remove('spinning'); btn.textContent = '失败'; btn.title = '详细原因请查看诊断日志'; btn.classList.add('is-err');
        try { await S.store.refreshProbeResults(); syncLatest(btn); } catch (_) { /* keep explicit request failure */ }
      }
      syncRouteTestButtons();
    }, disabled: !eligible || S.store.dirty, title: !eligible ? '已停用路由不能测试' : (S.store.dirty ? '请先保存或放弃工作副本修改' : label) }, label);
    btn._eligible = eligible;
    btn._label = label;
    btn._routeId = routeId;
    btn._download = download;
    btn._latest = h('small', { class: 'probe-latest' });
    syncLatest(btn);
    routeButtons.push(btn);
    return btn;
  }

  function kindCard(kind, title, desc, route) {
    const color = kind === 'direct' ? 'ok' : 'err';
    if (!route) {
      return h('section', { class: `card card--edge edge--${color}` }, [
        h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, kind === 'direct' ? 'Direct' : 'Reject'), h('div', { class: 'card__title' }, title))),
        h('p', { class: 'alert alert--err' }, `缺少必需的 ${kind === 'direct' ? 'Direct' : 'Reject'} 系统路由，请从高级配置恢复。`)
      ]);
    }
    const status = kind === 'direct'
      ? h('span', { class: 'badge badge--ok', title: 'Direct 是系统必需路由，始终启用' }, '固定启用')
      : ui.toggle(route.enabled, (v) => { route.enabled = v; S.store.touch(); });
    return h('section', { class: `card card--edge edge--${color}` }, [
      h('div', { class: 'card__head' }, [
        h('div', {}, h('span', { class: 'eyebrow' }, kind === 'direct' ? 'Direct · 必须恰好一个' : 'Reject'), h('div', { class: 'card__title' }, title)),
        status
      ]),
      h('p', { class: 'muted' }, desc)
    ]);
  }

  function openRouteEditor(route, focusOption) {
    const isNew = !S.store.intent.routes.includes(route);
    const opened = ui.drawer({
      eyebrow: isNew ? '新建路由' : '编辑路由', title: route.name || '未命名', submitLabel: '保存到工作副本',
      renderBody(body) {
        const intent = S.store.intent;
        const draft = JSON.parse(JSON.stringify(route));
        const name = ui.input({ value: draft.name || '', placeholder: '路由名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        draft.kind = 'single';
        const nodeOpts = ui.referenceOptions('nodes', intent.nodes);
        const nodeSel = ui.select(ui.selectWithMissing(nodeOpts, draft.node, '缺失节点'), draft.node ?? '', (v) => { draft.node = v; });
        const detourOpts = [['', '直连（无前置）'], ...ui.referenceOptions('routes', intent.routes.filter((r) => r.kind === 'single'))];
        const detourSel = ui.select(ui.selectWithMissing(detourOpts, draft.detour ?? ''), draft.detour ?? '', (v) => { draft.detour = v; });
        const warn = h('div', {});
        const check = () => {
          if (draft.detour && wouldCycle(draft.id, draft.detour, intent.routes)) {
            warn.replaceChildren(h('div', { class: 'alert alert--err' }, '前置代理链存在循环引用，请重新选择前置节点。'));
          } else if (draft.detour) {
            const d = intent.routes.find((r) => r.id === draft.detour);
            if (d && !d.enabled) warn.replaceChildren(h('div', { class: 'alert' }, '所选前置路由已停用，请启用或更换。'));
            else if (d && d.kind !== 'single') warn.replaceChildren(h('div', { class: 'alert' }, '前置必须是单节点路由。'));
            else warn.replaceChildren();
          } else {
            warn.replaceChildren();
          }
        };
        detourSel.addEventListener('change', check);
        nodeSel.addEventListener('change', () => { draft.node = nodeSel.value; });

        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '路由'), [
            ui.field('名称', name, null, 'name'),
            ui.field('启用', enabled, null, 'enabled'),
            ui.field('类型', h('span', { class: 'badge' }, '单节点'), '系统直连与拒绝路由不能从此处创建或转换'),
            h('div', { class: 'field--row' }, [
              ui.field('节点', nodeSel, '用于连接网络的出口节点', 'node'),
              ui.field('前置代理', detourSel, '先连接所选前置路由；留空表示直接连接', 'detour')
            ]),
            warn
          ])
        );
        return {
          submit() {
            draft.name = name.value.trim() || undefined;
            draft.kind = 'single';
            draft.node = nodeSel.value || undefined;
            draft.detour = detourSel.value || undefined;
            if (draft.detour && wouldCycle(draft.id, draft.detour, intent.routes)) { ui.toast('前置链存在循环引用，不能保存', 'err'); return false; }
            Object.assign(route, draft);
            return route;
          }
        };
      },
      onSubmit(route) {
        if (isNew) S.store.intent.routes.push(route);
        S.store.touch();
        ui.toast(`路由 ${route.name || route.id} 已${isNew ? '创建' : '更新'} · 未保存`, 'info');
        view.render(document.querySelector('#view'));
        return true;
      }
    });
    ui.focusDrawerOption(focusOption);
    return opened;
  }

  const view = {
    name: 'routes',
    render(root) {
      const isCurrent = ui.beginRender(root);
      routeButtons = [];
      const intent = S.store.intent;
      const direct = intent.routes.find((r) => r.kind === 'direct');
      const block = intent.routes.find((r) => r.kind === 'block');
      const singles = intent.routes.filter((r) => r.kind === 'single');

      const table = h('table', { class: 'table' }, [
        h('thead', {}, h('tr', {}, ['状态', '路由', '链路（出口 ← 前置）', '测试', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, singles.map((route) => h('tr', { class: route.enabled === false ? 'is-disabled' : null }, [
          h('td', {}, ui.toggle(route.enabled, (v) => { route.enabled = v; S.store.touch(); view.render(root); })),
          h('td', {}, h('strong', {}, route.name || route.id)),
          h('td', {}, chain(intent, route)),
          h('td', {}, h('div', { class: 'row-actions row-actions--probe' },
            probeAction(routeTestButton('链测试', false, route.id, route.enabled !== false)),
            probeAction(routeTestButton('下载', true, route.id, route.enabled !== false)))),
          h('td', {}, h('div', { class: 'row-actions' }, [
            h('button', { class: 'btn btn--sm', onclick: () => openRouteEditor(route) }, '编辑'),
            h('button', { class: 'btn btn--sm btn--danger', onclick: () => {
              if (!ui.guardCollectionDeletion('routes', route.id, route.name || route.id)) return;
              S.store.intent.routes = S.store.intent.routes.filter((r) => r.id !== route.id);
              S.store.touch();
              ui.toast(`已删除路由 ${route.name || route.id} · 未保存`, 'warn');
              view.render(root);
            } }, '删除')
          ]))
        ])))
      ]);

      syncRouteTestButtons();
      if (typeof S.store.subscribe === 'function') {
        const unsubscribe = S.store.subscribe(syncRouteTestButtons);
        isCurrent.onDispose(unsubscribe);
      }

      root.append(
        ui.viewHead('路由', '管理网络出站路由与前置代理链路', [
          h('button', {
            class: 'btn btn--primary',
            disabled: intent.nodes.length === 0,
            title: intent.nodes.length === 0 ? '请先添加节点' : '添加单节点路由',
            onclick: () => openRouteEditor(ui.creationDraft('routes', {
              node: intent.nodes.find((node) => node.enabled !== false)?.id || ''
            }))
          }, '添加单节点路由')
        ]),
        h('div', { class: 'grid-2' }, [
          kindCard('direct', 'Direct 直连', '匹配流量直接出网。启用配置必须恰好存在一个 Direct 路由。', direct),
          kindCard('block', 'Reject 拒绝', '匹配的流量将被直接拒绝连接。', block)
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '单节点路由'), h('div', { class: 'card__title' }, '节点链'))),
          h('p', { class: 'muted' }, '流量通过出口节点连接，支持指定前置代理节点先行建立连接。同一节点可在不同路由中配置不同前置链。'),
          singles.length ? h('div', { class: 'clipped' }, h('div', { class: 'table-wrap' }, table)) : h('div', { class: 'empty' }, '还没有单节点路由')
        ])
      );

      const focus = ui.takeObjectFocus('route');
      const focusedRoute = focus && intent.routes.find((route) => route.id === focus.object_id);
      if (focusedRoute?.kind === 'single') openRouteEditor(focusedRoute, focus.option);
      else if (focusedRoute) ui.toast(`已定位系统路由 ${focusedRoute.name || focusedRoute.id}；Direct/Reject 不可删除`, 'info');
    }
  };

  S.views = S.views || {};
  S.views.routes = view;
})();
