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
      description: '控制台仅允许本机访问。需要从其他设备打开时，可通过 SSH 建立临时转发：',
      command: `ssh -L ${port}:${listen} host\n# 然后访问 http://127.0.0.1:${port}`
    };
  }

  const view = {
    name: 'system',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      const [geosite, geoip] = await Promise.all([S.api.geodata('geosite'), S.api.geodata('geoip')]);
      const runtime = S.store.runtime || {};
      const status = S.store.overview?.status || {};
      const geo = runtime.geodata || {};
      const access = webAccess(runtime.web_listen);
      const lastApply = status.last_apply;
      const lastApplyResult = lastApply?.result;

      const geoCard = h('section', { class: 'card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '规则数据'), h('div', { class: 'card__title' }, 'GeoSite 与 GeoIP')),
          h('span', { class: geo.error ? 'badge badge--err' : 'badge badge--ok' }, geo.error ? '不可用' : '可用')
        ]),
        h('p', { class: 'muted' }, '规则数据随 Steer 提供，并会定期检查更新。'),
        h('dl', { class: 'facts' }, [
          fact('数据版本', runtimeValue(geo)),
          fact('规则总数', geo.rule_count == null ? '—' : String(geo.rule_count)),
          fact('GeoSite 规则数', geosite.readable ? String(geosite.count) : '不可用'),
          fact('GeoIP 规则数', geoip.readable ? String(geoip.count) : '不可用')
        ]),
        geo.error ? h('pre', { class: 'mono' }, geo.error) : null
      ]);

      const factsCard = h('section', { class: 'card' }, [
        h('span', { class: 'eyebrow' }, '版本'),
        h('div', { class: 'facts u-mt-10' }, [
          fact('Steer 版本', runtimeValue(runtime.steer)),
          fact('核心版本', runtimeValue(runtime.sing_box)),
          fact('上次应用', lastApply ? `${ui.applyTime(lastApply)} · ${lastApplyResult?.ok ? '成功' : '失败'}` : '—')
        ])
      ]);

      const platformCard = h('section', { class: 'card' }, [
        h('span', { class: 'eyebrow' }, '平台组件与路径'),
        h('div', { class: 'facts u-mt-10' }, [
          fact('服务', 'Steer · Web 控制台 · 订阅更新'),
          fact('配置', '/etc/steer/config.json'),
          fact('运行目录', '/run/steer'),
          fact('状态目录', '/var/lib/steer'),
          fact('规则数据', '/usr/share/steer/geodata-seed')
        ])
      ]);

      const accessCard = h('section', { class: 'card card--edge edge--warn' }, [
        h('span', { class: 'eyebrow' }, '访问'),
        h('p', { class: 'muted' }, access.description),
        h('pre', { class: 'mono command-block' }, access.command)
      ]);

      if (!isCurrent()) return;
      root.append(ui.viewHead('系统', '版本、规则数据与系统路径'), geoCard, factsCard, platformCard, accessCard);
    }
  };

  S.views = S.views || {};
  S.views.system = view;
})();
