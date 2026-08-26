/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 系统：包内 Geo seed、运行时版本与访问说明。 */
'use strict';
(function () {
  const S = window.S;
  const { h } = S;
  const ui = S.ui;

  function fact(label, value) {
    return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value));
  }

  function runtimeValue(value) {
    if (value && typeof value === 'object') {
      if (value.version) return value.version;
      return value.error ? '不可用' : '—';
    }
    return value == null || value === '' ? '—' : String(value);
  }

  function webAccess(listenValue) {
    const listen = String(listenValue || '');
    const match = listen.match(/^\[[^\]]+\]:(\d+)$/) || listen.match(/^[^:]+:(\d+)$/);
    if (!match) return {
      description: '运行时未返回可确认的 Web 监听地址；无法生成安全的 SSH 转发示例。',
      command: '监听地址不可用'
    };
    const port = match[1];
    return {
      description: `Web 实际监听 ${listen}，仅允许 loopback IP。远程访问使用 SSH 端口转发：`,
      command: `ssh -L ${port}:${listen} host\n# 然后访问 http://127.0.0.1:${port}（需要 web-token）`
    };
  }

  const view = {
    name: 'system',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      const [geosite, geoip] = await Promise.all([S.api.geodata('geosite'), S.api.geodata('geoip')]);
      const runtime = S.store.runtime || {};
      const geo = runtime.geodata || {};
      const access = webAccess(runtime.web_listen);

      const geoCard = h('section', { class: 'card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, 'Geo seed'), h('div', { class: 'card__title' }, '包内初始规则')),
          h('span', { class: geo.error ? 'badge badge--err' : 'badge badge--ok' }, geo.error ? '不可用' : '清单可用')
        ]),
        h('p', { class: 'muted' }, 'Apply 会校验所需 seed 并离线启动；sing-box 每 24 小时在后台检查远端 latest。'),
        h('dl', { class: 'facts' }, [
          fact('seed version', runtimeValue(geo)),
          fact('all rules', geo.rule_count == null ? '—' : String(geo.rule_count)),
          fact('GeoSite selectors', geosite.readable ? String(geosite.count) : '不可用'),
          fact('GeoIP categories', geoip.readable ? String(geoip.count) : '不可用')
        ]),
        geo.error ? h('pre', { class: 'mono' }, geo.error) : null
      ]);

      const factsCard = h('section', { class: 'card' }, [
        h('span', { class: 'eyebrow' }, '版本'),
        h('div', { class: 'facts u-mt-10' }, [
          fact('steer', runtimeValue(runtime.steer)),
          fact('sing-box', runtimeValue(runtime.sing_box)),
          fact('canonical schema', runtimeValue(runtime.canonical_schema)),
          fact('build tags', Array.isArray(runtime.sing_box?.tags) ? runtime.sing_box.tags.join(' / ') : '—')
        ])
      ]);

      const accessCard = h('section', { class: 'card card--edge edge--warn' }, [
        h('span', { class: 'eyebrow' }, '访问'),
        h('p', { class: 'muted' }, access.description),
        h('pre', { class: 'mono command-block' }, access.command)
      ]);

      if (!isCurrent()) return;
      root.append(ui.viewHead('系统', '包内 seed 与可确认的运行事实'), geoCard, factsCard, accessCard);
    }
  };

  S.views = S.views || {};
  S.views.system = view;
})();
