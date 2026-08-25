/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 节点：订阅分组 + 行内/批量测速 + 协议抽屉编辑 + 分享链接导入。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtReport, asList } = S;
  const ui = S.ui;

  const MANUAL = '_manual';
  let activeGroup = MANUAL;
  const rowButtons = new Map(); /* nodeId -> {conn, down} */

  const PROTOCOL_LABEL = Object.fromEntries(S.uiSpec.node_types.map((item) => [item.value, item.label]));
  const FIELD_LABEL = {
    uuid: 'UUID', username: '用户名', password: '密码', method: '加密方法', plugin: '插件', plugin_options: '插件参数',
    security: '加密', alter_id: 'Alter ID', network: 'Network', packet_encoding: 'UDP 包编码', flow: 'Flow',
    transport: '传输', transport_path: '传输路径', transport_host: '传输主机', service_name: 'gRPC 服务名',
    server_ports: '端口跳跃区间', hop_interval: '跳跃间隔', obfs_type: '混淆', obfs_password: '混淆密码',
    up_mbps: '上行 Mbps', down_mbps: '下行 Mbps', version: '版本', congestion_control: '拥塞控制',
    udp_relay_mode: 'UDP 中继', udp_over_stream: 'UDP over stream', zero_rtt_handshake: '0-RTT 握手', heartbeat: '心跳',
    quic: 'QUIC', quic_congestion_control: 'QUIC 拥塞控制', insecure_concurrency: '不安全并发', private_key: '私钥',
    host_key: 'Host key', host_key_algorithms: 'Host key algorithms', executable_path: '可执行文件', extra_args: '额外参数',
    data_directory: '数据目录', tls_server_name: 'TLS 服务器名', utls_fingerprint: 'uTLS 指纹',
    insecure: '跳过证书校验', reality_public_key: 'REALITY 公钥', reality_short_id: 'REALITY Short ID'
  };
  const NODE_OPTION_KEYS = new Set(S.uiSpec.node_fields.map((field) => field.key));
  function fieldsFor(type) { return S.uiSpec.node_fields.filter((field) => field.types.includes(type)); }

  function groupOf(node) { return node.source_subscription || MANUAL; }
  function groups(intent) {
    const subs = new Map(intent.subscriptions.map((s) => [s.id, s]));
    const list = [{ id: MANUAL, label: '手动节点', count: 0 }];
    const index = { [MANUAL]: list[0] };
    intent.nodes.forEach((node) => {
      const id = groupOf(node);
      if (!index[id]) {
        const sub = subs.get(id);
        index[id] = { id, label: sub ? (sub.name || id) : `缺失订阅：${id}`, count: 0 };
        list.push(index[id]);
      }
      index[id].count++;
    });
    return list;
  }

  function testButton(label, download, nodeId) {
    const btn = h('button', { class: 'btn btn--sm', title: label, onclick: async () => {
      if (S.store.dirty) {
        ui.toast('请先保存或放弃工作副本修改，再测试节点', 'warn');
        return;
      }
      btn.disabled = true;
      btn.classList.add('spinning');
      btn.textContent = '测试中…';
      try {
        const report = await S.api.speedtestNode(nodeId, download);
        const r = fmtReport(report, download);
        btn.classList.remove('spinning');
        btn.textContent = r.label;
        btn.title = r.detail;
        btn.classList.toggle('is-ok', r.ok);
        btn.classList.toggle('is-err', !r.ok);
      } catch (e) {
        btn.classList.remove('spinning');
        btn.textContent = '失败';
        btn.title = e.message;
        btn.classList.add('is-err');
      }
      btn.disabled = false;
    } }, label);
    return btn;
  }

  function renderBatch(nodes) {
    if (!nodes.length) return null;
    const bar = h('div', { class: 'batch-bar' });
    const make = (label, download) => {
      const btn = h('button', { class: 'btn', onclick: () => runBatch(nodes, download, btn, label) }, label);
      bar.append(btn);
      return btn;
    };
    make('批量连接测试', false);
    make('批量下载测试', true);
    return bar;
  }

  async function runBatch(ids, download, button, title) {
    if (S.store.dirty) {
      ui.toast('请先保存或放弃工作副本修改，再批量测试节点', 'warn');
      return;
    }
    const rows = ids.map((id) => rowButtons.get(id)).filter(Boolean);
    rows.forEach((pair) => { pair.conn.disabled = true; pair.down.disabled = true; });
    let cursor = 0, succeeded = 0;
    const worker = async () => {
      while (cursor < ids.length) {
        const id = ids[cursor++];
        const pair = rowButtons.get(id);
        const btn = download ? pair?.down : pair?.conn;
        if (btn) {
          btn.classList.add('spinning');
          btn.textContent = '…';
          try {
            const report = await S.api.speedtestNode(id, download);
            const r = fmtReport(report, download);
            btn.textContent = r.label;
            btn.title = r.detail;
            btn.classList.toggle('is-ok', r.ok);
            btn.classList.toggle('is-err', !r.ok);
            if (r.ok) succeeded++;
          } catch (e) { btn.textContent = '失败'; btn.title = e.message; btn.classList.add('is-err'); }
          btn.classList.remove('spinning');
        }
        button.textContent = `${title} · ${cursor}/${ids.length}`;
      }
    };
    await Promise.all(Array.from({ length: Math.min(4, ids.length) }, worker));
    button.textContent = `${title} · 成功 ${succeeded}/${ids.length}`;
    rows.forEach((pair) => { pair.conn.disabled = false; pair.down.disabled = false; });
  }

  /* ---------- 分享链接导入 ---------- */
  function openImport() {
    const textarea = h('textarea', { class: 'textarea', rows: 4, placeholder: 'ss://…  vmess://…  vless://…  trojan://…  hysteria2://…  tuic://…' });
    const preview = h('div', { class: 'import-preview' });
    let parsed = null;

    async function review() {
      try {
        parsed = await S.api.importNode(textarea.value);
        const node = parsed.node;
        const nameInput = ui.input({ value: node.name, placeholder: '节点名称' });
        const facts = [
          ['协议', PROTOCOL_LABEL[node.type] || node.type], ['服务器', node.server], ['端口', String(node.server_port)],
          ['凭据', '已解析（预览隐藏）'], ['证书校验', node.insecure ? '已禁用' : '启用']
        ];
        preview.replaceChildren(
          h('h4', { class: 'import-title' }, '确认导入节点'),
          ...(parsed.warnings.length ? [h('div', { class: 'alert' }, h('strong', {}, '解析警告'), h('ul', { class: 'import-warnings' }, parsed.warnings.map((w) => h('li', {}, w.detail))))] : []),
          ui.field('名称', nameInput),
          h('div', { class: 'facts' }, facts.map(([k, v]) => h('div', { class: 'fact' }, h('dt', {}, k), h('dd', {}, v)))),
          h('p', { class: 'muted' }, '不支持的字段会中止导入而不是静默丢弃。'),
          h('div', { class: 'dialog-inline-actions' }, h('button', {
            class: 'btn btn--primary', onclick: () => {
              node.name = nameInput.value.trim() || node.name;
              S.store.intent.nodes.push(node);
              S.store.touch();
              ui.toast('节点已加入工作副本 · 未保存', 'info');
              activeGroup = MANUAL;
              view.render(document.querySelector('#view'));
              close();
            }
          }, '添加到工作副本'))
        );
      } catch (e) {
        preview.replaceChildren(h('div', { class: 'alert alert--err' }, e.message));
      }
    }

    const { close } = ui.dialog({
      title: '导入分享链接',
      body: h('div', {}, [
        h('p', { class: 'muted' }, '解析在浏览器内完成，链接不写入日志；凭据预览打码，确认后才进入工作副本。'),
        textarea, preview,
        h('div', { class: 'dialog-inline-actions u-mt-10' }, [
          h('button', { class: 'btn', onclick: () => close() }, '取消'),
          h('button', { class: 'btn', onclick: review }, '解析并预览')
        ])
      ])
    });
  }

  /* ---------- 节点编辑抽屉 ---------- */
  function openNodeEditor(node) {
    const isNew = !S.store.intent.nodes.includes(node);
    ui.drawer({
      eyebrow: `节点 · ${node.id}`, title: node.name || '未命名', submitLabel: '保存到工作副本',
      renderBody(body) {
        const draft = JSON.parse(JSON.stringify(node));
        const name = ui.input({ value: draft.name || '', placeholder: '节点名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const typeSel = ui.select(S.uiSpec.node_types.map((item) => [item.value, item.label]), draft.type, (v) => { draft.type = v; rebuildSpec(); });
        const server = ui.input({ value: draft.server || '', placeholder: 'example.com 或 IP' });
        const port = ui.input({ type: 'number', value: draft.server_port || '', placeholder: '443' });
        const endpoint = h('div', { class: 'field--row' });
        const specBox = h('div', {});

        function specControl(field) {
          const key = field.key;
          if (field.control === 'boolean') return ui.toggleRow(FIELD_LABEL[key] || field.label, !!draft[key], (v) => { draft[key] = v; });
          if (field.control === 'string-list') return ui.chips(asList(draft[key]), { placeholder: field.placeholder || '', onchange: (v) => { draft[key] = v; } });
          if (field.control === 'select' || field.control === 'select-integer') return ui.select(
            field.options.map((item) => [item.value, item.label]), String(draft[key] ?? field.default ?? ''),
            (v) => { draft[key] = field.control === 'select-integer' && v !== '' ? Number(v) : v; rebuildSpec(); }
          );
          if (field.control === 'integer') return ui.input({ type: 'number', value: draft[key] ?? '', placeholder: field.placeholder || '', oninput: (e) => { draft[key] = e.target.value === '' ? undefined : Number(e.target.value); } });
          return ui.input({
            type: field.control === 'password' ? 'password' : 'text', value: draft[key] ?? '',
            placeholder: field.placeholder || '', oninput: (e) => { draft[key] = e.target.value; }
          });
        }

        function rebuildSpec() {
          specBox.replaceChildren();
          endpoint.replaceChildren();
          endpoint.hidden = draft.type === 'tor';
          if (draft.type !== 'tor') endpoint.append(ui.field('服务器', server), ui.field('端口', port));
          for (const field of fieldsFor(draft.type).filter((candidate) => !['enabled', 'name', 'server', 'server_port'].includes(candidate.key))) {
            if (field.when && !field.when.values.includes(String(draft[field.when.field] ?? ''))) continue;
            specBox.append(ui.field(
              FIELD_LABEL[field.key] || field.label,
              specControl(field),
              field.sensitive ? '敏感字段；留空表示不设置' : null
            ));
          }
        }
        rebuildSpec();

        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '基本信息'), [
            ui.field('名称', name), ui.field('启用', enabled),
            ui.field('协议', typeSel, '协议字段来自共享 UI 规格'),
            endpoint
          ]),
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '协议参数'), specBox)
        );

        return {
          submit() {
            const trim = (v) => String(v ?? '').trim();
            if (!trim(name.value)) { ui.toast('名称不能为空', 'err'); return false; }
            draft.name = trim(name.value);
            if (draft.type === 'tor') {
              delete draft.server;
              delete draft.server_port;
            } else {
              if (!trim(server.value)) { ui.toast('服务器不能为空', 'err'); return false; }
              const p = Number(port.value);
              if (!port.value || p < 1 || p > 65535) { ui.toast('端口必须是 1–65535', 'err'); return false; }
              draft.server = trim(server.value);
              draft.server_port = p;
            }
            const allowed = new Set(fieldsFor(draft.type).map((field) => field.key));
            for (const key of NODE_OPTION_KEYS) if (!allowed.has(key)) delete draft[key];
            for (const key of Object.keys(draft)) {
              if (draft[key] === '' || draft[key] == null) delete draft[key];
            }
            Object.assign(node, draft);
            return node;
          }
        };
      },
      onSubmit(node) {
        if (isNew) S.store.intent.nodes.push(node);
        S.store.touch();
        ui.toast(`节点 ${node.name} 已${isNew ? '创建' : '更新'} · 未保存`, 'info');
        view.render(document.querySelector('#view'));
        return true;
      }
    });
  }

  /* ---------- 视图 ---------- */
  const view = {
    name: 'nodes',
    render(root) {
      ui.beginRender(root);
      const intent = S.store.intent;
      rowButtons.clear();
      const groupList = groups(intent);
      if (!groupList.some((g) => g.id === activeGroup)) activeGroup = MANUAL;
      const active = groupList.find((g) => g.id === activeGroup);
      const nodes = intent.nodes.filter((n) => groupOf(n) === activeGroup);
      const editable = activeGroup === MANUAL;

      const groupNav = h('div', { class: 'node-groups' }, groupList.map((g) => h('button', {
        class: `chip ${g.id === activeGroup ? 'is-active' : ''}`,
        onclick: () => { activeGroup = g.id; view.render(root); }
      }, h('span', {}, g.label), h('span', { class: 'count' }, String(g.count)))));

      const table = h('table', { class: 'table' }, [
        h('thead', {}, h('tr', {}, ['状态', '节点', '协议', '服务器', '测试', '操作'].map((t) => h('th', {}, t)))),
        h('tbody', {}, nodes.map((node) => {
          const conn = testButton('连接', false, node.id);
          const down = testButton('下载', true, node.id);
          rowButtons.set(node.id, { conn, down });
          const edit = editable ? h('button', { class: 'btn btn--sm', onclick: () => openNodeEditor(node) }, '编辑') : h('span', { class: 'badge' }, '订阅');
          const del = editable ? h('button', { class: 'btn btn--sm btn--danger', onclick: () => {
            S.store.intent.nodes = S.store.intent.nodes.filter((n) => n.id !== node.id);
            S.store.touch();
            ui.toast(`已删除 ${node.name} · 未保存`, 'warn');
            view.render(root);
          } }, '删除') : null;
          return h('tr', { class: node.enabled === false ? 'is-disabled' : null }, [
            h('td', {}, ui.toggle(node.enabled, (v) => { node.enabled = v; S.store.touch(); })),
            h('td', {}, h('div', {}, h('div', {}, h('strong', {}, node.name || node.id)), h('div', { class: 'mono' }, node.id), node.pinned_stale ? h('span', { class: 'badge badge--stale' }, 'stale') : null)),
            h('td', {}, h('span', { class: `badge protocol-badge protocol--${node.type}` }, PROTOCOL_LABEL[node.type] || node.type)),
            h('td', { class: 'mono' }, `${node.server}:${node.server_port}`),
            h('td', {}, h('div', { class: 'row-actions' }, conn, down)),
            h('td', {}, h('div', { class: 'row-actions' }, edit, del))
          ]);
        }))
      ]);

      const batch = renderBatch(nodes.map((n) => n.id));
      root.append(
        ui.viewHead('节点', editable ? '手动维护的节点；订阅节点只读，由订阅更新生成' : '订阅生成数据 · 只读 · 修改请在订阅源或手动节点中完成', [
          h('button', { class: 'btn', onclick: openImport }, '导入节点'),
          editable ? h('button', { class: 'btn btn--primary', onclick: () => openNodeEditor({ id: S.uid('node'), enabled: true, name: '', type: 'trojan', server: '', server_port: 443 }) }, '添加节点') : null
        ]),
        groupNav
      );
      if (batch) root.append(batch);
      root.append(
        nodes.length ? h('div', { class: 'card table-card' }, h('div', { class: 'table-wrap' }, table)) : h('div', { class: 'empty' }, '该分组没有节点')
      );
    }
  };

  S.views = S.views || {};
  S.views.nodes = view;
})();
