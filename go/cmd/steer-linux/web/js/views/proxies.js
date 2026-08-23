/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 本地代理：规则 inbound 的入口端点。 */
'use strict';
(function () {
  const S = window.S;
  const { h, asList } = S;
  const ui = S.ui;

  function refCount(intent, proxyId) {
    return intent.rules.filter((r) => asList(r.inbound).includes(proxyId)).length;
  }

  function openEditor(proxy) {
    const isNew = !S.store.intent.local_proxies.includes(proxy);
    ui.drawer({
      eyebrow: `本地代理 · ${proxy.id}`, title: proxy.name || '未命名', submitLabel: '保存到工作副本',
      renderBody(body) {
        const draft = JSON.parse(JSON.stringify(proxy));
        const name = ui.input({ value: draft.name || '', placeholder: '端点名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const protocol = ui.select([['socks', 'SOCKS'], ['http', 'HTTP CONNECT']], draft.protocol, (v) => { draft.protocol = v; });
        const listen = ui.input({ value: draft.listen || '', placeholder: '127.0.0.1' });
        const port = ui.input({ type: 'number', value: draft.listen_port || '', placeholder: '1080' });
        const username = ui.input({ value: draft.username || '', placeholder: '（可选）' });
        const password = ui.input({ type: 'password', value: draft.password || '' });

        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '端点'), [
            ui.field('名称', name),
            ui.field('启用', enabled),
            h('div', { class: 'field--row' }, [ui.field('协议', protocol), ui.field('监听端口', port)]),
            ui.field('监听地址', listen, '通常保持环回地址'),
            h('div', { class: 'field--row' }, [
              ui.field('用户名', username),
              ui.field('密码', password, '留空 = 保留已保存值')
            ])
          ])
        );
        return {
          submit() {
            if (!name.value.trim()) { ui.toast('名称不能为空', 'err'); return false; }
            if (!listen.value.trim()) { ui.toast('监听地址不能为空', 'err'); return false; }
            const p = Number(port.value);
            if (!port.value || p < 1 || p > 65535) { ui.toast('端口必须是 1–65535', 'err'); return false; }
            draft.name = name.value.trim();
            draft.listen = listen.value.trim();
            draft.listen_port = p;
            if (!draft.username) delete draft.username;
            if (password.value) draft.password = password.value;
            Object.assign(proxy, draft);
            return proxy;
          }
        };
      },
      onSubmit(proxy) {
        if (isNew) S.store.intent.local_proxies.push(proxy);
        S.store.touch();
        ui.toast(`本地代理 ${proxy.name} 已${isNew ? '创建' : '更新'} · 未保存`, 'info');
        view.render(document.querySelector('#view'));
        return true;
      }
    });
  }

  const view = {
    name: 'proxies',
    render(root) {
      ui.beginRender(root);
      const intent = S.store.intent;
      const table = h('table', { class: 'table' }, [
        h('thead', {}, h('tr', {}, ['状态', '名称', '协议', '监听', '规则引用', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, intent.local_proxies.map((p) => h('tr', { class: p.enabled === false ? 'is-disabled' : null }, [
          h('td', {}, ui.toggle(p.enabled, (v) => { p.enabled = v; S.store.touch(); })),
          h('td', {}, h('div', {}, h('strong', {}, p.name || p.id), h('div', { class: 'mono' }, p.id))),
          h('td', {}, h('span', { class: 'badge badge--match' }, p.protocol === 'socks' ? 'SOCKS' : 'HTTP CONNECT')),
          h('td', { class: 'mono' }, `${p.listen}:${p.listen_port}`),
          h('td', { class: 'mono num' }, String(refCount(intent, p.id))),
          h('td', {}, h('div', { class: 'row-actions' }, [
            h('button', { class: 'btn btn--sm', onclick: () => openEditor(p) }, '编辑'),
            h('button', { class: 'btn btn--sm btn--danger', onclick: () => {
              S.store.intent.local_proxies = S.store.intent.local_proxies.filter((x) => x.id !== p.id);
              S.store.touch();
              ui.toast(`已删除 ${p.name} · 未保存`, 'warn');
              view.render(root);
            } }, '删除')
          ]))
        ])))
      ]);

      root.append(
        ui.viewHead('本地代理', '规则可用 inbound 把匹配限制到指定端点；无法与源 MAC 组合（平台级限制）', [
          h('button', { class: 'btn btn--primary', onclick: () => openEditor({ id: S.uid('proxy'), enabled: true, name: '', protocol: 'socks', listen: '127.0.0.1', listen_port: 1080 }) }, '添加端点')
        ]),
        intent.local_proxies.length
          ? h('section', { class: 'card table-card' }, h('div', { class: 'table-wrap' }, table))
          : h('div', { class: 'empty' }, '还没有本地代理端点')
      );
    }
  };

  S.views = S.views || {};
  S.views.proxies = view;
})();
