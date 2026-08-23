/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 系统：Geo 数据路径（Linux 平台设置）+ 版本事实 + 访问说明。 */
'use strict';
(function () {
  const S = window.S;
  const { h } = S;
  const ui = S.ui;

  function intentRequiresGeo(intent, kind) {
    const prefix = kind === 'geosite' ? 'geosite:' : 'geoip:';
    return intent.rules.some((rule) => rule.enabled && [
      ...(kind === 'geosite' ? rule.domain_match || [] : rule.ip_match || [])
    ].some((expr) => String(expr).startsWith(prefix)));
  }

  function geoRow(kind, label, status) {
    const intent = S.store.intent;
    const input = ui.input({ value: S.store.platform[`${kind}_path`] || '', placeholder: `/absolute/path/${kind}.dat`, oninput: (e) => { S.store.platform[`${kind}_path`] = e.target.value.trim(); } });
    const required = intentRequiresGeo(intent, kind);
    let badge;
    if (status.readable) badge = h('span', { class: 'badge badge--ok' }, `${status.count} categories`);
    else if (status.configured) badge = h('span', { class: 'badge badge--err' }, '不可读');
    else badge = h('span', { class: 'badge' }, '未配置');
    return h('div', { class: 'field' }, [
      h('label', {}, label),
      h('div', { class: 'field-inline' }, input, badge, required ? h('span', { class: 'badge badge--warn' }, '当前规则需要') : null),
      h('div', { class: 'field__hint' }, status.error ? `错误：${status.error.message || status.error}` : (required ? '启用规则引用了该 kind，文件必须存在且可读' : '当前未被启用规则引用，可不配置'))
    ]);
  }

  const view = {
    name: 'system',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      const [geosite, geoip] = await Promise.all([S.api.geodata('geosite'), S.api.geodata('geoip')]);
      const runtime = S.store.runtime || {};

      const geoCard = h('section', { class: 'card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '平台设置'), h('div', { class: 'card__title' }, 'Geo 数据路径')),
          h('span', { class: 'badge', title: S.store.platformRevision }, `平台修订 ${S.fmtRevision(S.store.platformRevision)}`)
        ]),
        h('p', { class: 'muted' }, 'Geo 路径属于 Linux 平台设置，不进入 Canonical Intent；保存后立即 Apply。'),
        geoRow('geosite', 'GeoSite database', geosite),
        geoRow('geoip', 'GeoIP database', geoip),
        h('button', { class: 'btn btn--primary', onclick: async () => {
          try {
            const res = await S.store.savePlatform();
            if (res.applied) ui.toast(`平台设置已保存并 Apply · ${S.fmtRevision(res.revision)}`, 'ok');
            else ui.toast(`平台设置已保存，但 Apply 失败：${res.error?.message || res.apply_result?.error || '运行态未切换'}`, 'err');
            view.render(root);
          } catch (error) {
            ui.toast(`平台设置保存失败：${error.message}`, 'err');
          }
        } }, '保存并 Apply')
      ]);

      const factsCard = h('section', { class: 'card' }, [
        h('span', { class: 'eyebrow' }, '版本'),
        h('div', { class: 'facts u-mt-10' }, [
          fact('steer', runtimeValue(runtime.steer)),
          fact('sing-box', runtimeValue(runtime.sing_box, '（<1.14.0）')),
          fact('geoview', runtimeValue(runtime.geoview)),
          fact('canonical schema', runtimeValue(runtime.canonical_schema)),
          fact('platform schema', runtimeValue(runtime.platform_schema)),
          fact('build tags', runtimeTags(runtime.sing_box))
        ])
      ]);

      const accessCard = h('section', { class: 'card card--edge edge--warn' }, [
        h('span', { class: 'eyebrow' }, '访问'),
        h('p', { class: 'muted' }, 'Web 只监听 127.0.0.1:9080，禁止绑定公网。远程访问使用 SSH 端口转发：'),
        h('pre', { class: 'mono command-block' }, 'ssh -L 9080:127.0.0.1:9080 host\n# 然后访问 http://127.0.0.1:9080（需要 web-token）')
      ]);

      function fact(label, value) { return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value)); }
      function runtimeValue(value, suffix = '') {
        if (value && typeof value === 'object') {
          if (value.version) return `${value.version}${suffix}`;
          return value.error ? '不可用' : '—';
        }
        return value == null || value === '' ? '—' : `${value}${suffix}`;
      }
      function runtimeTags(tool) {
        const tags = Array.isArray(tool?.tags) ? tool.tags : [];
        return tags.length ? tags.join(' / ') : (tool?.error ? '不可用' : '—');
      }

      if (!isCurrent()) return;
      root.append(
        ui.viewHead('系统', 'Linux 平台设置与版本信息；配置语义仍在 Canonical Intent 中'),
        geoCard, factsCard, accessCard
      );
    }
  };

  S.views = S.views || {};
  S.views.system = view;
})();
