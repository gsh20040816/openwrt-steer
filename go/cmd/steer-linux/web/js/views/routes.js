/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 路由：Direct/Block 固定语义卡 + single 节点链（node ← detour 链）与环检测。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtReport } = S;
  const ui = S.ui;

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
    return h('span', { class: 'chain', title: '前置代理先拨号' }, parts, hops.length > 1 ? h('span', { class: 'muted' }, '出口') : null);
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

  function routeTestButton(label, download, routeId) {
    const btn = h('button', { class: 'btn btn--sm', title: label, onclick: async () => {
      if (S.store.dirty) {
        ui.toast('请先保存或放弃工作副本修改，再测试路由', 'warn');
        return;
      }
      btn.disabled = true; btn.classList.add('spinning'); btn.textContent = '测试中…';
      try {
        const report = await S.api.speedtestRoute(routeId, download);
        const r = fmtReport(report, download);
        btn.classList.remove('spinning'); btn.textContent = r.label; btn.title = r.detail;
        btn.classList.toggle('is-ok', r.ok); btn.classList.toggle('is-err', !r.ok);
      } catch (e) { btn.classList.remove('spinning'); btn.textContent = '失败'; btn.title = e.message; btn.classList.add('is-err'); }
      btn.disabled = false;
    } }, label);
    return btn;
  }

  function kindCard(kind, title, desc, route) {
    const color = kind === 'direct' ? 'ok' : 'err';
    const toggle = ui.toggle(route.enabled, (v) => { route.enabled = v; S.store.touch(); });
    return h('section', { class: `card card--edge edge--${color}` }, [
      h('div', { class: 'card__head' }, [
        h('div', {}, h('span', { class: 'eyebrow' }, kind === 'direct' ? 'Direct · 必须恰好一个' : 'Block'), h('div', { class: 'card__title' }, title)),
        toggle
      ]),
      h('p', { class: 'muted' }, desc)
    ]);
  }

  function openRouteEditor(route) {
    const isNew = !S.store.intent.routes.includes(route);
    ui.drawer({
      eyebrow: `路由 · ${route.id}`, title: route.name || '未命名', submitLabel: '保存到工作副本',
      renderBody(body) {
        const intent = S.store.intent;
        const draft = JSON.parse(JSON.stringify(route));
        const name = ui.input({ value: draft.name || '', placeholder: '路由名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const kind = ui.select(S.uiSpec.route_kinds.map((item) => [item.value, item.label]), draft.kind, (v) => { draft.kind = v; });
        const nodeOpts = intent.nodes.map((n) => [n.id, `${n.name || n.id}（${n.type}）`]);
        const nodeSel = ui.select(ui.selectWithMissing(nodeOpts, draft.node, '缺失节点'), draft.node ?? '', (v) => { draft.node = v; });
        const detourOpts = [['', '直连（无前置）'], ...intent.routes.filter((r) => r.kind === 'single').map((r) => [r.id, r.name || r.id])];
        const detourSel = ui.select(ui.selectWithMissing(detourOpts, draft.detour ?? ''), draft.detour ?? '', (v) => { draft.detour = v; });
        const warn = h('div', {});
        const check = () => {
          if (draft.detour && wouldCycle(draft.id, draft.detour, intent.routes)) {
            warn.replaceChildren(h('div', { class: 'alert alert--err' }, '会形成 detour 环（ROUTE_DETOUR_CYCLE）。Apply 会拒绝该配置。'));
          } else if (draft.detour) {
            const d = intent.routes.find((r) => r.id === draft.detour);
            if (d && !d.enabled) warn.replaceChildren(h('div', { class: 'alert' }, '前置路由已禁用；Apply 会拒绝。'));
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
            ui.field('名称', name),
            ui.field('启用', enabled),
            ui.field('类型', kind),
            h('div', { class: 'field--row' }, [
              ui.field('节点', nodeSel, 'single 路由的出站节点'),
              ui.field('前置代理（detour）', detourSel, '前置路由先拨号；留空直连')
            ]),
            warn
          ])
        );
        return {
          submit() {
            if (!name.value.trim()) { ui.toast('名称不能为空', 'err'); return false; }
            draft.name = name.value.trim();
            draft.kind = kind.value;
            draft.node = draft.kind === 'single' ? (nodeSel.value || undefined) : undefined;
            draft.detour = draft.kind === 'single' && detourSel.value ? detourSel.value : undefined;
            if (draft.detour && wouldCycle(draft.id, draft.detour, intent.routes)) { ui.toast('detour 链成环，不能保存', 'err'); return false; }
            if (draft.kind !== 'single') delete draft.node;
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
  }

  const view = {
    name: 'routes',
    render(root) {
      ui.beginRender(root);
      const intent = S.store.intent;
      const direct = intent.routes.find((r) => r.kind === 'direct');
      const block = intent.routes.find((r) => r.kind === 'block');
      const singles = intent.routes.filter((r) => r.kind === 'single');

      const table = h('table', { class: 'table' }, [
        h('thead', {}, h('tr', {}, ['状态', '路由', '链路（出口 ← 前置）', '测试', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, singles.map((route) => h('tr', { class: route.enabled === false ? 'is-disabled' : null }, [
          h('td', {}, ui.toggle(route.enabled, (v) => { route.enabled = v; S.store.touch(); })),
          h('td', {}, h('div', {}, h('strong', {}, route.name || route.id), h('div', { class: 'mono' }, route.id))),
          h('td', {}, chain(intent, route)),
          h('td', {}, h('div', { class: 'row-actions' }, routeTestButton('链测试', false, route.id), routeTestButton('下载', true, route.id))),
          h('td', {}, h('div', { class: 'row-actions' }, [
            h('button', { class: 'btn btn--sm', onclick: () => openRouteEditor(route) }, '编辑'),
            h('button', { class: 'btn btn--sm btn--danger', onclick: () => {
              S.store.intent.routes = S.store.intent.routes.filter((r) => r.id !== route.id);
              S.store.touch();
              ui.toast(`已删除路由 ${route.name || route.id} · 未保存`, 'warn');
              view.render(root);
            } }, '删除')
          ]))
        ])))
      ]);

      root.append(
        ui.viewHead('路由', 'Direct / Block 是固定语义；single 路由支持任意深度的前置链，但不得成环', [
          h('button', { class: 'btn btn--primary', onclick: () => openRouteEditor({ id: S.uid('route'), enabled: true, name: '', kind: 'single', node: '', detour: '' }) }, '添加路由')
        ]),
        h('div', { class: 'grid-2' }, [
          kindCard('direct', 'Direct 直连', '匹配流量直接出网。启用配置必须恰好存在一个 Direct 路由。', direct),
          kindCard('block', 'Block 阻断', '匹配流量被拒绝。', block)
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '单节点路由'), h('div', { class: 'card__title' }, '节点链'))),
          h('p', { class: 'muted' }, '应用流量 → 出口节点 → 前置节点 → 网络。同一节点可在不同路由中拥有不同前置链。'),
          singles.length ? h('div', { class: 'clipped' }, h('div', { class: 'table-wrap' }, table)) : h('div', { class: 'empty' }, '还没有 single 路由')
        ])
      );
    }
  };

  S.views = S.views || {};
  S.views.routes = view;
})();
