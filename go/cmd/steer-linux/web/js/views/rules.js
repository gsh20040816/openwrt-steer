/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 规则：有序 first-match 列表（拖拽排序）+ 系统直连固定卡 + Default 固定卡 + Match 编辑器。 */
'use strict';
(function () {
  const S = window.S;
  const { h, asList } = S;
  const ui = S.ui;

  let catalogPromise = null;
  function catalogs() {
    if (!catalogPromise) {
      catalogPromise = Promise.all([S.api.geodata('geosite'), S.api.geodata('geoip')])
        .then(([geosite, geoip]) => ({ geosite, geoip }));
    }
    return catalogPromise;
  }

  function routeLabel(intent, id) {
    const r = intent.routes.find((x) => x.id === id);
    return r ? (r.name || id) : id;
  }
  function dnsLabel(intent, id) {
    const p = intent.dns_profiles.find((x) => x.id === id);
    return p ? (p.name || id) : id;
  }

  function summaryTokens(rule) {
    if (rule.default) return ['default'];
    const tokens = [];
    for (const field of S.uiSpec.rule_match_fields) {
      const values = asList(rule[field]);
      if (!values.length) continue;
      tokens.push((field === 'network' || field === 'protocol')
        ? `${field}:${values.join('/')}` : `${field}:${values.length}`);
    }
    return tokens;
  }

  function dnsContinues(rule) {
    const populated = S.uiSpec.rule_match_fields.filter((field) => asList(rule[field]).length > 0);
    return populated.length > 0 && populated.every((field) => S.uiSpec.rule_connection_only_fields.includes(field));
  }

  function summary(rule) {
    const labels = {
      inbound: 'inbound', domain_match: '域名', ip_match: 'IP', source_ip_cidr: '源 CIDR',
      source_mac_address: '源 MAC', network: '网络', protocol: '协议', port: '端口'
    };
    const parts = summaryTokens(rule).map((token) => {
      if (token === 'default') return '兜底';
      const separator = token.indexOf(':');
      const field = token.slice(0, separator);
      const value = token.slice(separator + 1);
      if (field === 'network') return value.toUpperCase();
      if (field === 'protocol') return `协议 ${value.replace(/\//g, ',')}`;
      return `${labels[field] || field} ×${value}`;
    });
    if (!parts.length) parts.push('无匹配条件');
    if (dnsContinues(rule)) parts.push('DNS 继续匹配后续规则');
    return parts.join(' · ');
  }

  async function openRuleEditor(rule, focusOption) {
    const isNew = !S.store.intent.rules.includes(rule);
    const geo = await catalogs();
    const opened = ui.drawer({
      eyebrow: `规则 · ${rule.id}`, title: rule.name || '未命名', submitLabel: '保存到工作副本', width: 560,
      renderBody(body) {
        const intent = S.store.intent;
        const draft = JSON.parse(JSON.stringify(rule));
        const name = ui.input({ value: draft.name || '', placeholder: '规则名称' });
        const enabled = ui.toggle(draft.enabled, (v) => { draft.enabled = v; });
        const dnsOpts = intent.dns_profiles.map((p) => [p.id, p.name || p.id]);
        const dnsSel = ui.select(ui.selectWithMissing(dnsOpts, draft.dns_profile, '缺失 Profile'), draft.dns_profile, (v) => { draft.dns_profile = v; });
        const routeOpts = intent.routes.map((r) => [r.id, r.name || r.id]);
        const routeSel = ui.select(ui.selectWithMissing(routeOpts, draft.route, '缺失路由'), draft.route, (v) => { draft.route = v; });

        const inboundChoices = asList(intent.local_proxies).map((proxy) => [proxy.id, proxy.name || proxy.id]);
        const inboundControl = ui.multiChoice(inboundChoices, asList(draft.inbound), {
          onchange: (v) => { draft.inbound = v; }, missingLabel: '缺失本地代理'
        });
        const domain = ui.matchEditor({
          value: draft.domain_match, kind: 'domain', catalog: geo.geosite?.names || [],
          placeholder: 'domain:example.com\nfull:www.example.com\nkeyword\ngeosite:geolocation-!cn\nregexp:^api\\d+\\.example\\.com$'
        });
        const ip = ui.matchEditor({
          value: draft.ip_match, kind: 'ip', catalog: geo.geoip?.names || [],
          placeholder: '1.1.1.1\n10.0.0.0/8\ngeoip:cn'
        });
        const srcCidr = ui.chips(asList(draft.source_ip_cidr), { placeholder: '192.168.50.0/24', onchange: (v) => { draft.source_ip_cidr = v; } });
        const srcMac = ui.chips(asList(draft.source_mac_address), { placeholder: '02:00:00:00:00:10', onchange: (v) => { draft.source_mac_address = v; } });
        const nets = ui.multiChoice(S.uiSpec.rule_networks.map((item) => [item.value, item.label]), asList(draft.network), { onchange: (v) => { draft.network = v; } });
        const protos = ui.multiChoice(S.uiSpec.rule_protocols.map((item) => [item.value, item.label]), asList(draft.protocol), { onchange: (v) => { draft.protocol = v; } });
        const ports = ui.chips(asList(draft.port).map(String), { placeholder: '443 / 27015', onchange: (v) => { draft.port = v.map((x) => (/^\d+$/.test(x) ? Number(x) : x)); } });

        body.append(
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '意图'), [
            ui.field('名称', name, null, 'name'),
            ui.field('启用', enabled, null, 'enabled'),
            h('div', { class: 'field--row' }, [ui.field('DNS Profile', dnsSel, null, 'dns_profile'), ui.field('路由', routeSel, null, 'route')])
          ]),
          h('div', { class: 'drawer-section' }, h('div', { class: 'drawer-section__title' }, '匹配条件 · 同字段 OR · 跨字段 AND'), [
            ui.field('inbound（本地代理端点）', inboundControl, '只能选择当前本地代理；悬空引用会保留供修复', 'inbound'),
            ui.field('域名匹配', domain.el, `每行一条 · ${geo.geosite?.readable ? `${geo.geosite.count} 个 GeoSite 名称可补全` : 'catalog 不可用，Apply 时最终判定'}`, 'domain_match'),
            ui.field('目标 IP 匹配', ip.el, `每行一条 · ${geo.geoip?.readable ? `${geo.geoip.count} 个 GeoIP 名称可补全` : 'catalog 不可用，Apply 时最终判定'} · 只参与连接阶段`, 'ip_match'),
            ui.field('源 IP/CIDR', srcCidr, '稳定 DHCP 租约或稳定 IPv6 地址', 'source_ip_cidr'),
            ui.field('源 MAC', srcMac, '由 sing-box 1.14 邻居解析原生匹配；不能与本地代理 inbound 组合', 'source_mac_address'),
            h('div', { class: 'field--row' }, [
              ui.field('网络', nets, '仅连接阶段', 'network'),
              ui.field('检测协议', protos, '仅连接阶段', 'protocol')
            ]),
            ui.field('目标端口', ports, '精确端口 · 仅连接阶段', 'port'),
            h('div', { class: 'alert' }, 'DNS Profile 只使用 inbound、域名、源 IP/CIDR 与源 MAC 条件。只有 IP/network/protocol/port 时，DNS 会继续匹配后续规则。')
          ])
        );
        return {
          submit() {
            [srcCidr, srcMac, ports].forEach((control) => control.commitPending());
            if (!name.value.trim()) { ui.toast('名称不能为空', 'err'); return false; }
            draft.name = name.value.trim();
            draft.dns_profile = dnsSel.value;
            draft.route = routeSel.value;
            draft.domain_match = domain.value;
            draft.ip_match = ip.value;
            for (const key of Object.keys(draft)) if (draft[key] == null || (Array.isArray(draft[key]) && draft[key].length === 0)) delete draft[key];
            Object.assign(rule, draft);
            return rule;
          }
        };
      },
      onSubmit(rule) {
        if (isNew) {
          const rules = S.store.intent.rules;
          const defaultIndex = rules.findIndex((candidate) => candidate.default);
          rules.splice(defaultIndex < 0 ? rules.length : defaultIndex, 0, rule);
        }
        S.store.touch();
        ui.toast(`规则 ${rule.name} 已${isNew ? '创建' : '更新'} · 未保存`, 'info');
        view.render(document.querySelector('#view'));
        return true;
      }
    });
    ui.focusDrawerOption(focusOption);
    return opened;
  }

  function renderRow(rule, index) {
    const intent = S.store.intent;
    const row = h('div', { class: `rule-row ${rule.enabled === false ? 'is-disabled' : ''}`, draggable: 'true', dataset: { ruleId: rule.id } }, [
      h('span', { class: 'grip', title: '拖拽排序' }, '⠿'),
      h('span', { class: 'rule-row__order' }, String(index + 1).padStart(2, '0')),
      h('div', { class: 'rule-row__name' }, h('strong', {}, rule.name || rule.id), h('span', { class: 'rule-row__summary' }, summary(rule))),
      h('div', { class: 'rule-row__intent' }, [
        h('span', { class: 'badge badge--dns' }, dnsLabel(intent, rule.dns_profile)),
        h('span', { class: 'badge badge--route' }, routeLabel(intent, rule.route))
      ]),
      h('div', { class: 'rule-row__actions' }, [
        h('button', { class: 'btn btn--sm', onclick: () => openRuleEditor(rule) }, '编辑'),
        h('button', { class: 'btn btn--sm btn--danger', onclick: () => {
          if (!ui.guardCollectionDeletion('rules', rule.id, rule.name || rule.id)) return;
          S.store.intent.rules = S.store.intent.rules.filter((r) => r.id !== rule.id);
          S.store.touch();
          ui.toast(`已删除规则 ${rule.name} · 未保存`, 'warn');
          view.render(document.querySelector('#view'));
        } }, '删除')
      ])
    ]);
    row.addEventListener('dragstart', (e) => {
      row.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', rule.id);
    });
    row.addEventListener('dragend', () => { row.classList.remove('dragging'); document.querySelectorAll('.rule-row').forEach((r) => r.classList.remove('drag-over')); });
    row.addEventListener('dragover', (e) => {
      e.preventDefault();
      row.classList.add('drag-over');
    });
    row.addEventListener('dragleave', () => row.classList.remove('drag-over'));
    row.addEventListener('drop', (e) => {
      e.preventDefault();
      row.classList.remove('drag-over');
      const fromId = e.dataTransfer.getData('text/plain');
      if (!fromId || fromId === rule.id) return;
      const rules = S.store.intent.rules;
      const from = rules.findIndex((r) => r.id === fromId);
      const to = rules.findIndex((r) => r.id === rule.id);
      if (from < 0 || to < 0 || rules[from].default || rules[to].default) return;
      const [moved] = rules.splice(from, 1);
      rules.splice(to, 0, moved);
      S.store.touch();
      ui.toast('规则顺序已调整 · 未保存', 'info');
      view.render(document.querySelector('#view'));
    });
    return row;
  }

  const view = {
    name: 'rules',
    render(root) {
      ui.beginRender(root);
      const intent = S.store.intent;
      const ordered = intent.rules.filter((r) => !r.default);
      const defaultRule = intent.rules.find((r) => r.default);

      const sysCard = h('section', { class: 'card sys-card' }, [
        h('div', { class: 'sys-card__order' }, h('span', {}, '规则 1 之前'), h('strong', {}, '系统')),
        h('div', {}, [
          h('strong', {}, '系统直连'),
          h('p', {}, '非全球可达与本地目的地址在用户规则之前放行：环回、私网、链路本地等。传统 TCP/UDP 53 请求进入 DNS shim。'),
          h('details', { class: 'fold' }, [
            h('summary', {}, '查看固定边界'),
            h('p', {}, '环回、私有、共享、链路本地、文档、基准、丢弃与组播范围。全球可达的特殊用途地址仍可参与用户规则。')
          ])
        ]),
        h('span', { class: 'badge badge--dns' }, 'DIRECT')
      ]);

      const list = h('div', { class: 'rule-list' }, ordered.map((rule, i) => renderRow(rule, i)));
      if (!ordered.length) list.append(h('div', { class: 'empty' }, '还没有普通规则；Default 规则兜底'));

      const defaultCard = h('section', { class: 'card default-card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, `第 ${ordered.length + 1} 条 · 始终启用`), h('div', { class: 'card__title' }, 'Default')),
          h('span', { class: 'badge badge--match' }, '只有 DNS Profile 与路由可修改')
        ]),
        h('div', { class: 'default-fields' }, [
          ui.field('DNS Profile', ui.select(intent.dns_profiles.map((p) => [p.id, p.name || p.id]), defaultRule.dns_profile, (v) => { defaultRule.dns_profile = v; S.store.touch(); }), null, 'dns_profile'),
          ui.field('路由', ui.select(intent.routes.map((r) => [r.id, r.name || r.id]), defaultRule.route, (v) => { defaultRule.route = v; S.store.touch(); }), null, 'route')
        ])
      ]);

      root.append(
        ui.viewHead('规则', 'first-match 有序执行；拖拽调整顺序。启用配置必须恰好一个 Default 规则', [
          h('button', { class: 'btn btn--primary', onclick: () => {
            const rule = { id: S.uid('rule'), enabled: true, name: '新规则', dns_profile: intent.dns_profiles[0]?.id || '', route: 'direct' };
            openRuleEditor(rule);
          } }, '添加规则')
        ]),
        sysCard,
        list,
        defaultCard
      );

      const focus = ui.takeObjectFocus('rule');
      const focusedRule = focus && intent.rules.find((rule) => rule.id === focus.object_id);
      if (focusedRule?.default) ui.focusDrawerOption(focus.option);
      else if (focusedRule) void openRuleEditor(focusedRule, focus.option);
    }
  };

  view.summaryTokens = summaryTokens;
  view.dnsContinues = dnsContinues;

  S.views = S.views || {};
  S.views.rules = view;
})();
