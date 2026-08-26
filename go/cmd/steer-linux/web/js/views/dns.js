/* SPDX-License-Identifier: GPL-3.0-or-later */
/* DNS Profile：六种传输 + TLS + 规则引用计数。 */
'use strict';
(function () {
  const S = window.S;
  const { h } = S;
  const ui = S.ui;

  const PROTOCOL_LABEL = Object.fromEntries(S.uiSpec.dns_protocols.map((item) => [item.value, item.label]));
  const DNS_FIELDS = new Set(S.uiSpec.dns_protocols.flatMap((item) => item.fields));
  const DEFAULT_PROTOCOL = S.uiSpec.dns_protocols[0];

  function protocolSpec(value) {
    return S.uiSpec.dns_protocols.find((item) => item.value === value);
  }

  function cleanupProtocolFields(draft) {
    const spec = protocolSpec(draft.protocol);
    if (!spec) return;
    for (const field of DNS_FIELDS) if (!spec.fields.includes(field)) delete draft[field];
  }

  function applyProtocol(draft, nextValue) {
    const previous = protocolSpec(draft.protocol);
    const next = protocolSpec(nextValue);
    if (!next) return draft;
    const currentPort = Number(draft.server_port);
    if (!Number.isInteger(currentPort) || currentPort < 1 || (previous && currentPort === previous.default_port)) {
      draft.server_port = next.default_port;
    }
    draft.protocol = next.value;
    cleanupProtocolFields(draft);
    return draft;
  }

  function commitProfile(profile, draft) {
    for (const field of DNS_FIELDS) if (!(field in draft)) delete profile[field];
    Object.assign(profile, draft);
    return profile;
  }

  function refCount(intent, profileId) {
    return intent.rules.filter((r) => r.dns_profile === profileId).length;
  }

  function openEditor(profile, focusOption) {
    const isNew = !S.store.intent.dns_profiles.includes(profile);
    const opened = ui.drawer({
      eyebrow: `DNS Profile · ${profile.id}`, title: profile.name || '未命名', submitLabel: '保存到工作副本',
      renderBody(body) {
        const draft = JSON.parse(JSON.stringify(profile));
        const initialSpec = protocolSpec(draft.protocol) || DEFAULT_PROTOCOL;
        draft.protocol = initialSpec.value;
        if (!Number.isInteger(Number(draft.server_port)) || Number(draft.server_port) < 1) draft.server_port = initialSpec.default_port;
        cleanupProtocolFields(draft);
        const name = ui.input({ value: draft.name || '', placeholder: 'Profile 名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const server = ui.input({ value: draft.server || '', placeholder: 'dns.example.com 或 IP' });
        const port = ui.input({ type: 'number', value: draft.server_port || '', placeholder: '53 / 853 / 443' });
        const protocolFields = h('div', {});
        const protocol = ui.select(S.uiSpec.dns_protocols.map((item) => [item.value, item.label]), draft.protocol, (v) => {
          draft.server_port = Number(port.value);
          applyProtocol(draft, v);
          port.value = draft.server_port;
          renderProtocolFields();
        });

        function renderProtocolFields() {
          const spec = protocolSpec(draft.protocol);
          const fields = [];
          for (const field of spec?.fields || []) {
            switch (field) {
            case 'tls_server_name':
              fields.push(ui.field('TLS 服务器名', ui.input({
                value: draft.tls_server_name || '', placeholder: 'dns.example.com',
                oninput: (event) => { draft.tls_server_name = event.target.value; }
              }), '加密 DNS 必填', 'tls_server_name'));
              break;
            case 'path':
              fields.push(ui.field('HTTP 路径', ui.input({
                value: draft.path || '', placeholder: '/dns-query',
                oninput: (event) => { draft.path = event.target.value; }
              }), 'DoH / DoH3 使用', 'path'));
              break;
            case 'insecure':
              fields.push(ui.field('跳过证书校验', ui.toggle(draft.insecure, (v) => { draft.insecure = v; }), null, 'insecure'));
              break;
            }
          }
          protocolFields.replaceChildren(...fields);
        }
        renderProtocolFields();

        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '上游'), [
            ui.field('名称', name, null, 'name'),
            ui.field('启用', enabled, null, 'enabled'),
            ui.field('协议', protocol, null, 'protocol'),
            h('div', { class: 'field--row' }, [ui.field('服务器', server, null, 'server'), ui.field('端口', port, null, 'server_port')]),
            protocolFields
          ])
        );
        return {
          submit() {
            if (!name.value.trim()) { ui.toast('名称不能为空', 'err'); return false; }
            if (!server.value.trim()) { ui.toast('服务器不能为空', 'err'); return false; }
            draft.name = name.value.trim();
            draft.server = server.value.trim();
            draft.server_port = Number(port.value);
            if (!Number.isInteger(draft.server_port) || draft.server_port < 1 || draft.server_port > 65535) { ui.toast('端口必须是 1–65535', 'err'); return false; }
            for (const field of ['path', 'tls_server_name']) {
              if (typeof draft[field] === 'string') draft[field] = draft[field].trim() || undefined;
            }
            cleanupProtocolFields(draft);
            const spec = protocolSpec(draft.protocol);
            const missing = spec.required_fields.find((field) => !String(draft[field] || '').trim());
            if (missing) { ui.toast('加密 DNS 需要 TLS 服务器名', 'err'); return false; }
            for (const key of Object.keys(draft)) if (draft[key] == null) delete draft[key];
            return commitProfile(profile, draft);
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
    ui.focusDrawerOption(focusOption);
    return opened;
  }

  const view = {
    name: 'dns',
    render(root) {
      ui.beginRender(root);
      const intent = S.store.intent;
      const table = h('table', { class: 'table' }, [
        h('thead', {}, h('tr', {}, ['状态', '名称', '协议', '服务器', '规则引用', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, intent.dns_profiles.map((p) => h('tr', { class: p.enabled === false ? 'is-disabled' : null }, [
          h('td', {}, ui.toggle(p.enabled, (v) => { p.enabled = v; S.store.touch(); })),
          h('td', {}, h('div', {}, h('strong', {}, p.name || p.id), h('div', { class: 'mono' }, p.id))),
          h('td', {}, h('span', { class: 'badge badge--dns' }, PROTOCOL_LABEL[p.protocol] || p.protocol)),
          h('td', { class: 'mono' }, `${p.server}:${p.server_port}${p.path ? p.path : ''}`),
          h('td', { class: 'mono num' }, String(refCount(intent, p.id))),
          h('td', {}, h('div', { class: 'row-actions' }, [
            h('button', { class: 'btn btn--sm', onclick: () => openEditor(p) }, '编辑'),
            h('button', { class: 'btn btn--sm btn--danger', onclick: () => {
              if (!ui.guardCollectionDeletion('dns_profiles', p.id, p.name || p.id)) return;
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
		  h('button', { class: 'btn btn--primary', onclick: () => openEditor({ id: S.uid('dns'), enabled: true, name: '', protocol: DEFAULT_PROTOCOL.value, server: '', server_port: DEFAULT_PROTOCOL.default_port }) }, '添加 Profile')
		]),
        intent.dns_profiles.length
          ? h('section', { class: 'card table-card' }, h('div', { class: 'table-wrap' }, table))
          : h('div', { class: 'empty' }, '还没有 DNS Profile')
      );
      const focus = ui.takeObjectFocus('dns_profile');
      const focused = focus && intent.dns_profiles.find((profile) => profile.id === focus.object_id);
      if (focused) openEditor(focused, focus.option);
    }
  };

  view.applyProtocol = applyProtocol;
  view.commitProfile = commitProfile;

  S.views = S.views || {};
  S.views.dns = view;
})();
