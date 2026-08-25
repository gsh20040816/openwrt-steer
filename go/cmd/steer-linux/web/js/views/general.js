/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 基础设置：Main、探测、DNS cache 与 Bootstrap 的原生表单。 */
'use strict';
(function () {
  const S = window.S;
  const { h } = S;
  const ui = S.ui;

  function textField(object, key, placeholder) {
    return ui.input({
      value: object[key] ?? '', placeholder: placeholder || '',
      oninput: (event) => { object[key] = event.target.value; S.store.touch(); }
    });
  }

  function numberField(object, key, placeholder) {
    return ui.input({
      type: 'number', value: object[key] ?? '', placeholder: placeholder || '',
      oninput: (event) => {
        const value = event.target.value;
        if (value === '') delete object[key]; else object[key] = Number(value);
        S.store.touch();
      }
    });
  }

  const view = {
    name: 'general',
    render(root) {
      ui.beginRender(root);
      const main = S.store.intent.main;
      const bootstrap = S.store.intent.bootstrap;
      const logLevel = ui.select(S.uiSpec.log_levels.map((item) => [item.value, item.label]), main.log_level, (value) => {
        main.log_level = value; S.store.touch();
      });
      const bootstrapProtocol = ui.select(S.uiSpec.bootstrap_protocols.map((item) => [item.value, item.label]), bootstrap.protocol, (value) => {
        bootstrap.protocol = value; S.store.touch();
      });
      const strategy = ui.select(S.uiSpec.bootstrap_strategies.map((item) => [item.value, item.label]), bootstrap.strategy, (value) => {
        bootstrap.strategy = value; S.store.touch();
      });

      root.append(
        ui.viewHead('基础设置', 'Main、手动探测、共享 DNS cache 与 Bootstrap DNS'),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '运行'), h('div', { class: 'card__title' }, '核心设置'))),
          ui.field('日志级别', logLevel),
          h('p', { class: 'muted' }, '启用状态使用页面顶部开关；切换会立即保存并 Apply。')
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '连通性'), h('div', { class: 'card__title' }, '探测目标'))),
          ui.field('直连探测 URL', textField(main, 'probe_direct', 'https://www.example.com/')),
          ui.field('代理探测 URL', textField(main, 'probe_proxy', 'https://www.example.com/')),
          ui.field('代理测速 URL', textField(main, 'speedtest_proxy', 'https://speed.example.com/file')),
          h('p', { class: 'muted' }, '必须是无凭据、无 fragment 的 HTTPS URL。')
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'DNS cache'), h('div', { class: 'card__title' }, '共享缓存'))),
          ui.field('缓存容量', numberField(main, 'dns_cache_capacity', '4096'), '0 或留空使用运行时默认值；自定义范围 1,024–10,000,000'),
          ui.field('持久化缓存', ui.toggle(main.dns_cache_persist, (value) => { main.dns_cache_persist = value; S.store.touch(); })),
          ui.field('乐观缓存', ui.toggle(main.dns_optimistic_cache, (value) => { main.dns_optimistic_cache = value; S.store.touch(); }))
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'Bootstrap'), h('div', { class: 'card__title' }, '启动解析器'))),
          ui.field('协议', bootstrapProtocol),
          h('div', { class: 'field--row' }, [
            ui.field('服务器 IP', textField(bootstrap, 'server', '1.1.1.1'), '必须是 IP literal，避免解析环路'),
            ui.field('端口', numberField(bootstrap, 'server_port', '53'))
          ]),
          ui.field('地址策略', strategy)
        ])
      );
    }
  };

  S.views = S.views || {};
  S.views.general = view;
})();
