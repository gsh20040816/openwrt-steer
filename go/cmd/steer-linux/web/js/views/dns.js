/* SPDX-License-Identifier: GPL-3.0-or-later */
/* DNS Profile：六种传输 + TLS + 缓存（1.14 预留标记）+ 规则引用计数。 */
'use strict';
(function () {
  const S = window.S;
  const { h } = S;
  const ui = S.ui;

  const PROTOCOL_LABEL = { udp: 'UDP', tcp: 'TCP', tls: 'DoT', https: 'DoH', quic: 'DoQ', h3: 'DoH3' };

  function refCount(intent, profileId) {
    return intent.rules.filter((r) => r.dns_profile === profileId).length;
  }

  function openEditor(profile) {
    const isNew = !S.store.intent.dns_profiles.includes(profile);
    ui.drawer({
      eyebrow: `DNS Profile · ${profile.id}`, title: profile.name || '未命名', submitLabel: '保存到工作副本',
      renderBody(body) {
        const draft = JSON.parse(JSON.stringify(profile));
        const name = ui.input({ value: draft.name || '', placeholder: 'Profile 名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const protocol = ui.select(Object.entries(PROTOCOL_LABEL).map(([v, l]) => [v, l]), draft.protocol, (v) => { draft.protocol = v; });
        const server = ui.input({ value: draft.server || '', placeholder: 'dns.example.com 或 IP' });
        const port = ui.input({ type: 'number', value: draft.server_port || '', placeholder: '53 / 853 / 443' });
        const strategy = ui.select([['prefer_ipv4', 'prefer_ipv4'], ['prefer_ipv6', 'prefer_ipv6'], ['ipv4_only', 'ipv4_only'], ['ipv6_only', 'ipv6_only']], draft.strategy || 'prefer_ipv4', (v) => { draft.strategy = v; });
        const path = ui.input({ value: draft.path || '', placeholder: '/dns-query' });
        const tlsName = ui.input({ value: draft.tls_server_name || '', placeholder: 'dns.example.com' });
        const insecure = ui.toggle(draft.insecure, (v) => { draft.insecure = v; });
        const cachePersist = ui.toggle(draft.cache_persist, (v) => { draft.cache_persist = v; });
        const optimistic = ui.toggle(draft.optimistic_cache, (v) => { draft.optimistic_cache = v; });

        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '上游'), [
            ui.field('名称', name),
            ui.field('启用', enabled),
            h('div', { class: 'field--row' }, [ui.field('协议', protocol), ui.field('地址策略', strategy)]),
            h('div', { class: 'field--row' }, [ui.field('服务器', server), ui.field('端口', port)]),
            ui.field('HTTP 路径', path, 'DoH / DoH3 使用'),
            ui.field('TLS 服务器名', tlsName, 'DoT / DoH / DoQ / DoH3 使用'),
            ui.field('跳过证书校验', insecure)
          ]),
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '缓存'), [
            ui.field('持久缓存', cachePersist, 'sing-box 1.14 预留 · 当前 1.13 基线拒绝'),
            ui.field('乐观缓存', optimistic, 'sing-box 1.14 预留 · 当前 1.13 基线拒绝')
          ])
        );
        return {
          submit() {
            if (!name.value.trim()) { ui.toast('名称不能为空', 'err'); return false; }
            if (!server.value.trim()) { ui.toast('服务器不能为空', 'err'); return false; }
            draft.name = name.value.trim();
            draft.server = server.value.trim();
            draft.server_port = Number(port.value);
            draft.strategy = strategy.value;
            draft.path = path.value.trim() || undefined;
            draft.tls_server_name = tlsName.value.trim() || undefined;
            draft.cache_persist = draft.cache_persist || undefined;
            draft.optimistic_cache = draft.optimistic_cache || undefined;
            for (const key of Object.keys(draft)) if (draft[key] == null) delete draft[key];
            Object.assign(profile, draft);
            return profile;
          }
        };
      },
      onSubmit(profile) {
        if (isNew) S.store.intent.dns_profiles.push(profile);
        S.store.touch();
        ui.toast(`DNS Profile ${profile.name} 已${isNew ? '创建' : '更新'} · 未保存`, 'info');
        view.render(document.querySelector('#view'));
        return true;
      }
    });
  }

  const view = {
    name: 'dns',
    render(root) {
      ui.beginRender(root);
      const intent = S.store.intent;
      const table = h('table', { class: 'table' }, [
        h('thead', {}, h('tr', {}, ['状态', '名称', '协议', '服务器', '地址策略', '规则引用', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, intent.dns_profiles.map((p) => h('tr', { class: p.enabled === false ? 'is-disabled' : null }, [
          h('td', {}, ui.toggle(p.enabled, (v) => { p.enabled = v; S.store.touch(); })),
          h('td', {}, h('div', {}, h('strong', {}, p.name || p.id), h('div', { class: 'mono' }, p.id))),
          h('td', {}, h('span', { class: 'badge badge--dns' }, PROTOCOL_LABEL[p.protocol] || p.protocol)),
          h('td', { class: 'mono' }, `${p.server}:${p.server_port}${p.path ? p.path : ''}`),
          h('td', { class: 'mono' }, p.strategy),
          h('td', { class: 'mono num' }, String(refCount(intent, p.id))),
          h('td', {}, h('div', { class: 'row-actions' }, [
            h('button', { class: 'btn btn--sm', onclick: () => openEditor(p) }, '编辑'),
            h('button', { class: 'btn btn--sm btn--danger', onclick: () => {
              S.store.intent.dns_profiles = S.store.intent.dns_profiles.filter((x) => x.id !== p.id);
              S.store.touch();
              ui.toast(`已删除 ${p.name} · 未保存`, 'warn');
              view.render(root);
            } }, '删除')
          ]))
        ])))
      ]);

      root.append(
        ui.viewHead('DNS Profile', '每个实际使用的 (规则, Profile) 拥有独立传输路径；被规则引用后不可悬空', [
          h('button', { class: 'btn btn--primary', onclick: () => openEditor({ id: S.uid('dns'), enabled: true, name: '', protocol: 'udp', server: '', server_port: 53, strategy: 'prefer_ipv4' }) }, '添加 Profile')
        ]),
        intent.dns_profiles.length
          ? h('section', { class: 'card table-card' }, h('div', { class: 'table-wrap' }, table))
          : h('div', { class: 'empty' }, '还没有 DNS Profile')
      );
    }
  };

  S.views = S.views || {};
  S.views.dns = view;
})();
