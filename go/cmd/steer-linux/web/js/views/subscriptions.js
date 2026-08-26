/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 订阅：状态表 + 立即更新 + stale 节点清理（引用安全由后端拒绝）+ 编辑抽屉。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtTime } = S;
  const ui = S.ui;

  let statuses = null;

  function latestFailure(status) {
    const failure = status.last_failure;
    if (!failure) return null;
    if (!status.last_success || !failure.at) return failure;
    return new Date(failure.at).getTime() > new Date(status.last_success).getTime() ? failure : null;
  }

  function statusLabel(status) {
    if (!status.enabled) return ['已停用', ''];
    if (latestFailure(status)) return [status.never_fetched ? '抓取失败' : '最近失败', 'badge--err'];
    if (status.never_fetched) return ['未抓取', ''];
    if (status.skipped) return ['成功 · 有跳过', 'badge--warn'];
    return ['成功', 'badge--ok'];
  }

  function updateSummary(status) {
    return `added ${status.added || 0} · current ${status.current || 0} · stale ${(status.stale || []).length} · skipped ${status.skipped || 0}`;
  }

  function requestDelete(subscription) {
    const intent = S.store.intent;
    const owned = intent.nodes.filter((node) => node.source_subscription === subscription.id);
    if (!ui.guardCollectionDeletion('subscriptions', subscription.id, subscription.name || subscription.id)) return;
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

  function openEditor(sub, focusOption) {
    const isNew = !sub;
    const opened = ui.drawer({
      eyebrow: `订阅 · ${isNew ? '新建' : sub.id}`, title: sub?.name || '新订阅', submitLabel: '保存到工作副本',
      renderBody(body) {
        const defaultInterval = S.uiSpec.subscription_update_interval_default;
        const draft = sub ? JSON.parse(JSON.stringify(sub)) : { id: '', enabled: true, name: '', url: '', update_interval: defaultInterval };
        const id = ui.input({ value: draft.id, placeholder: 'acme', disabled: !isNew });
        const name = ui.input({ value: draft.name || '', placeholder: '订阅名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const url = ui.input({ value: draft.url || '', placeholder: 'https://sub.example.com/all' });
        const interval = ui.input({ value: draft.update_interval || '', placeholder: defaultInterval });
        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '订阅'), [
            ui.field('ID', id, isNew ? '1–32 位小写字母开头（稳定 ID，创建后不可改）' : '稳定 ID · 不可修改', 'id'),
            ui.field('名称', name, null, 'name'),
            ui.field('启用', enabled, null, 'enabled'),
            ui.field('订阅 URL', url, null, 'url'),
            ui.field('更新间隔', interval, '定时调度使用 · 留空表示仅手动更新', 'update_interval')
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
    ui.focusDrawerOption(focusOption);
    return opened;
  }

  function openCleanup(subscription, nodes, isCurrent, root) {
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
          if (!ui.guardCollectionDeletion('nodes', node.id, node.name || node.id)) return;
          if (S.store.dirty) {
            ui.toast('请先保存或放弃工作副本修改，再清理 stale 节点', 'warn');
            return;
          }
          try {
            const startedEpoch = S.store.draftEpoch;
            await S.api.cleanNode(subscription.id, node.id);
            let reloaded = false;
            if (S.store.draftEpoch !== startedEpoch && S.store.dirty) {
              await S.store.refreshOverview();
              ui.toast(`已清理服务器库存中的 ${node.id}；期间 Draft 已变化，已保留本地修改`, 'warn');
            } else {
              const reload = await S.store.reload();
              reloaded = reload?.ok === true;
              if (!reloaded) ui.toast(`已清理服务器库存中的 ${node.id}；Draft 未自动 reload`, 'warn');
            }
            if (reloaded) {
              const index = nodes.indexOf(node);
              if (index >= 0) nodes.splice(index, 1);
              ui.toast(`已清理 ${node.id}`, 'ok');
            }
            renderList();
            if (isCurrent()) view.render(root);
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
        h('thead', {}, h('tr', {}, ['状态', '订阅', 'URL', '间隔', '最近成功', '最近失败', '节点', 'skipped / stale', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, statuses.map((s) => {
          const state = statusLabel(s);
          const updateBtn = h('button', { class: 'btn btn--sm', onclick: async () => {
            if (!s.enabled) {
              ui.toast('已停用的订阅不能更新；请先启用并保存配置', 'warn');
              return;
            }
            if (S.store.dirty) {
              ui.toast('请先保存或放弃工作副本修改，再更新订阅', 'warn');
              return;
            }
            updateBtn.disabled = true;
            updateBtn.classList.add('spinning');
            updateBtn.textContent = '更新中…';
            try {
              const startedEpoch = S.store.draftEpoch;
              const res = await S.api.updateSubscription(s.id);
              const updated = (res.subscriptions || []).find((item) => item.id === s.id);
              let reloaded = false;
              if (S.store.draftEpoch !== startedEpoch && S.store.dirty) {
                await S.store.refreshOverview();
                ui.toast('节点库存已更新；期间 Draft 已变化，已保留本地修改且未自动 reload', 'warn');
              } else {
                const reload = await S.store.reload();
                reloaded = reload?.ok === true;
                if (!reloaded) ui.toast('节点库存已更新；Draft 未自动 reload，请稍后重试', 'warn');
              }
              if (reloaded) ui.toast(updated
                ? `节点库存已更新 · ${updateSummary(updated)}；运行配置未改变，未自动 Apply`
                : '节点库存已更新；运行配置未改变，未自动 Apply', 'warn');
              updateBtn.textContent = '立即更新';
              if (isCurrent()) await view.render(root);
            } catch (e) {
              ui.toast(e.message, 'err');
              updateBtn.textContent = '立即更新';
              if (isCurrent()) view.render(root);
            }
            updateBtn.classList.remove('spinning');
            updateBtn.disabled = !s.enabled;
          }, disabled: !s.enabled, title: s.enabled ? '' : '已停用的订阅不能更新' }, '立即更新');
          const cleanupBtn = s.stale?.length
            ? h('button', {
              class: 'btn btn--sm btn--danger',
                onclick: () => openCleanup(s, intent.nodes.filter((n) => n.source_subscription === s.id), isCurrent, root)
              }, `清理 stale ×${s.stale.length}`)
            : null;
          const failure = s.last_failure;
          return h('tr', { class: s.enabled === false ? 'is-disabled' : null }, [
            h('td', {}, h('div', { class: 'row-actions' }, [
              ui.toggle(s.enabled, (v) => { const sub = S.store.intent.subscriptions.find((x) => x.id === s.id); sub.enabled = v; S.store.touch(); }),
              h('span', { class: `badge ${state[1]}` }, state[0])
            ])),
            h('td', {}, h('div', {}, h('strong', {}, s.name || s.id), h('div', { class: 'mono' }, s.id))),
            h('td', { class: 'mono' }, h('span', { title: s.url }, s.url.length > 34 ? s.url.slice(0, 34) + '…' : s.url)),
            h('td', { class: 'mono' }, s.update_interval || '—'),
            h('td', { class: 'mono' }, s.last_success ? fmtTime(s.last_success) : h('span', { class: 'muted' }, '—')),
            h('td', {}, failure ? h('div', {}, [
              h('div', { class: 'mono' }, failure.at ? fmtTime(failure.at) : '—'),
              h('div', { class: 'muted', title: failure.summary }, failure.summary)
            ]) : h('span', { class: 'muted' }, '—')),
            h('td', { class: 'mono num' }, `${s.node_count} (${s.current} current)`),
            h('td', { class: 'mono num' }, `${s.skipped || 0} / ${(s.stale || []).length}`),
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
      const focus = ui.takeObjectFocus('subscription');
      const focused = focus && intent.subscriptions.find((subscription) => subscription.id === focus.object_id);
      if (focused) openEditor(focused, focus.option);
    }
  };

  S.views = S.views || {};
  S.views.subscriptions = view;
})();
