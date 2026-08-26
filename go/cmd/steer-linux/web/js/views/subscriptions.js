/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 订阅：状态表 + 立即更新 + stale 节点清理（引用安全由后端拒绝）+ 编辑抽屉。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtTime } = S;
  const ui = S.ui;

  let statuses = null;

  function requestDelete(subscription) {
    const intent = S.store.intent;
    const owned = intent.nodes.filter((node) => node.source_subscription === subscription.id);
    const ownedIDs = new Set(owned.map((node) => node.id));
    const referenced = intent.routes.filter((route) => ownedIDs.has(route.node));
    if (referenced.length) {
      ui.toast(`不能删除：${referenced.length} 条路由仍引用该订阅节点`, 'err');
      return;
    }
    ui.dialog({
      title: `删除订阅 · ${subscription.name || subscription.id}`,
      body: h('p', {}, `将从工作副本删除订阅和 ${owned.length} 个订阅节点。保存前可重新加载恢复。`),
      actions: [
        ['取消', null],
        ['删除', (close) => {
          intent.subscriptions = intent.subscriptions.filter((item) => item.id !== subscription.id);
          intent.nodes = intent.nodes.filter((node) => node.source_subscription !== subscription.id);
          S.store.touch();
          ui.toast(`已删除 ${subscription.name || subscription.id} · 未保存`, 'warn');
          close();
          view.render(document.querySelector('#view'));
        }, 'btn--danger']
      ]
    });
  }

  function openEditor(sub) {
    const isNew = !sub;
    ui.drawer({
      eyebrow: `订阅 · ${isNew ? '新建' : sub.id}`, title: sub?.name || '新订阅', submitLabel: '保存到工作副本',
      renderBody(body) {
        const draft = sub ? JSON.parse(JSON.stringify(sub)) : { id: '', enabled: true, name: '', url: '', update_interval: '12h' };
        const id = ui.input({ value: draft.id, placeholder: 'acme', disabled: !isNew });
        const name = ui.input({ value: draft.name || '', placeholder: '订阅名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const url = ui.input({ value: draft.url || '', placeholder: 'https://sub.example.com/all' });
        const interval = ui.input({ value: draft.update_interval || '', placeholder: '12h' });
        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '订阅'), [
            ui.field('ID', id, isNew ? '1–32 位小写字母开头（稳定 ID，创建后不可改）' : '稳定 ID · 不可修改'),
            ui.field('名称', name),
            ui.field('启用', enabled),
            ui.field('订阅 URL', url),
            ui.field('更新间隔', interval, 'systemd timer 使用 · 留空表示不自动更新')
          ])
        );
        return {
          submit() {
            if (!/^[a-z][a-z0-9_]{0,31}$/.test(id.value.trim())) { ui.toast('ID 必须 1–32 位小写字母开头', 'err'); return false; }
            if (!name.value.trim()) { ui.toast('名称不能为空', 'err'); return false; }
            if (!/^https?:\/\//.test(url.value.trim())) { ui.toast('订阅 URL 必须是 http(s)://', 'err'); return false; }
            draft.id = id.value.trim();
            draft.name = name.value.trim();
            draft.url = url.value.trim();
            draft.update_interval = interval.value.trim() || undefined;
            if (isNew) S.store.intent.subscriptions.push(draft);
            else Object.assign(sub, draft);
            return draft;
          }
        };
      },
      onSubmit(sub) {
        S.store.touch();
        ui.toast(`订阅 ${sub.name} 已${isNew ? '创建' : '更新'} · 未保存`, 'info');
        view.render(document.querySelector('#view'));
        return true;
      }
    });
  }

  function openCleanup(subscription, nodes) {
    const box = h('div', {});
    const renderList = () => {
      box.replaceChildren();
      const remaining = nodes.filter((n) => n.pinned_stale);
      if (!remaining.length) {
        box.replaceChildren(h('p', { class: 'muted' }, '没有 stale 节点。'));
        return;
      }
      box.append(h('ul', { class: 'issue-list' }, remaining.map((node) => h('li', { class: 'issue issue--warning' }, [
        h('span', { class: 'issue__code' }, 'SUBSCRIPTION_NODE_STALE'),
        h('span', { class: 'issue__where' }, node.id),
        h('span', { class: 'issue__msg' }, node.name || node.id),
        h('button', { class: 'btn btn--sm btn--danger', onclick: async () => {
          if (S.store.dirty) {
            ui.toast('请先保存或放弃工作副本修改，再清理 stale 节点', 'warn');
            return;
          }
          try {
            await S.api.cleanNode(subscription.id, node.id);
            await S.store.reload();
            const index = nodes.indexOf(node);
            if (index >= 0) nodes.splice(index, 1);
            ui.toast(`已清理 ${node.id}`, 'ok');
            renderList();
          } catch (e) {
            ui.toast(`${e.code || 'ERROR'}: ${e.message}`, 'err');
          }
        } }, '移除')
      ]))));
    };
    renderList();
    ui.dialog({
      title: `清理 stale 节点 · ${subscription.name || subscription.id}`,
      body: h('div', {}, [
        h('p', { class: 'muted' }, 'stale 节点是订阅更新后仍被 pin 保留的过期节点。仍被路由引用的节点会被后端拒绝清理（NODE_STILL_REFERENCED）。'),
        box
      ]),
      actions: [['关闭', null]]
    });
  }

  const view = {
    name: 'subscriptions',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      const intent = S.store.intent;
      statuses = (await S.api.subscriptions()).subscriptions;

      const table = h('table', { class: 'table' }, [
        h('thead', {}, h('tr', {}, ['状态', '订阅', 'URL', '间隔', '上次更新', '节点', 'stale', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, statuses.map((s) => {
          const updateBtn = h('button', { class: 'btn btn--sm', onclick: async () => {
            if (S.store.dirty) {
              ui.toast('请先保存或放弃工作副本修改，再更新订阅', 'warn');
              return;
            }
            updateBtn.disabled = true;
            updateBtn.classList.add('spinning');
            updateBtn.textContent = '更新中…';
            try {
              const res = await S.api.updateSubscription(s.id);
              const snap = res.snapshots?.[0];
              await S.store.reload();
              ui.toast(snap?.skipped ? `更新完成 · ${snap.node_count} 节点 · 跳过 ${snap.skipped} 个无效节点` : '更新完成', 'ok');
              updateBtn.textContent = '立即更新';
              view.render(root);
            } catch (e) {
              ui.toast(e.message, 'err');
              updateBtn.textContent = '立即更新';
            }
            updateBtn.classList.remove('spinning');
            updateBtn.disabled = false;
          } }, '立即更新');
          const cleanupBtn = s.stale_node_ids?.length
            ? h('button', {
                class: 'btn btn--sm btn--danger',
                onclick: () => openCleanup(s, intent.nodes.filter((n) => n.source_subscription === s.id))
              }, `清理 stale ×${s.stale_node_ids.length}`)
            : null;
          return h('tr', { class: s.enabled === false ? 'is-disabled' : null }, [
            h('td', {}, ui.toggle(s.enabled, (v) => { const sub = S.store.intent.subscriptions.find((x) => x.id === s.id); sub.enabled = v; S.store.touch(); })),
            h('td', {}, h('div', {}, h('strong', {}, s.name || s.id), h('div', { class: 'mono' }, s.id))),
            h('td', { class: 'mono' }, h('span', { title: s.url }, s.url.length > 34 ? s.url.slice(0, 34) + '…' : s.url)),
            h('td', { class: 'mono' }, s.update_interval || '—'),
            h('td', { class: 'mono' }, s.fetched_at ? fmtTime(s.fetched_at) : h('span', { class: 'muted' }, s.error ? '更新失败' : '未抓取')),
            h('td', { class: 'mono num' }, String(s.node_count)),
            h('td', { class: 'mono num' }, s.stale_node_ids?.length ? h('span', { class: 'badge badge--stale' }, String(s.stale_node_ids.length)) : '0'),
            h('td', {}, h('div', { class: 'row-actions row-actions--wrap' }, [
              updateBtn,
              cleanupBtn,
              h('button', { class: 'btn btn--sm', onclick: () => openEditor(S.store.intent.subscriptions.find((x) => x.id === s.id)) }, '编辑'),
              h('button', { class: 'btn btn--sm btn--danger', onclick: () => requestDelete(s) }, '删除')
            ]))
          ]);
        }))
      ]);

      if (!isCurrent()) return;
      root.append(
        ui.viewHead('订阅', 'systemd timer 只更新配置，不自动 Apply；更新失败保留旧配置', [
          h('button', { class: 'btn btn--primary', onclick: () => openEditor(null) }, '添加订阅')
        ]),
        statuses.length
          ? h('section', { class: 'card table-card' }, h('div', { class: 'table-wrap' }, table))
          : h('div', { class: 'empty' }, '还没有订阅')
      );
    }
  };

  S.views = S.views || {};
  S.views.subscriptions = view;
})();
