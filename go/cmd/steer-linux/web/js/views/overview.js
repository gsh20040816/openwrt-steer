/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 总览：Draft / Saved / Active、规模、warnings 与少量快捷操作。 */
'use strict';
(function () {
  const S = window.S;
  const { h } = S;
  const ui = S.ui;

  function step(kind, num, title, sub) {
    return h('div', { class: `step step--${kind}` }, [
      num != null ? h('span', { class: 'step__num' }, num) : null,
      h('strong', {}, title),
      h('small', {}, sub)
    ]);
  }
  const arrow = () => h('span', { class: 'arrow', 'aria-hidden': 'true' }, '→');

  const view = {
    name: 'overview',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      const intent = S.store.intent;
      const ov = S.store.overview || {};
      const status = ov.status || {};
      const lastApply = status.last_apply || null;
      const lastResult = lastApply?.result || lastApply;
      const validation = await S.api.validate(intent);
      const healthy = !!status.healthy;
      const externalChange = S.store.hasExternalChange === true;

      const externalNotice = externalChange ? h('section', { class: 'card card--edge edge--err' }, [
        h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'Revision'), h('div', { class: 'card__title' }, '服务器 Saved 配置已变化'))),
        h('p', {}, S.store.dirty
          ? '当前 Draft 已保留且不会自动覆盖。请先保存、放弃或在顶部显式处理外部变更冲突。'
          : '当前 Draft 仍是旧 Saved 基线。点击顶部“重载最新 Saved”即可安全更新对象列表。')
      ]) : null;

      const pipeline = h('section', { class: 'card hero' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '执行模型'), h('div', { class: 'card__title' }, '流量如何被转向')),
          h('span', { class: 'badge badge--match' }, 'first-match · 严格有序')
        ]),
        h('div', { class: 'pipeline' }, [
          step('match', intent.rules.length, '匹配规则', '首条命中即停'),
          arrow(),
          step('dns', intent.dns_profiles.length, 'DNS Profile', '每个 (规则, Profile) 独立路径'),
          arrow(),
          step('route', intent.routes.length, '路由', 'Direct / Reject / 节点链'),
          arrow(),
          step('net', null, '网络出口', '前置代理先拨号')
        ])
      ]);

      const summary = h('section', { class: 'card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '状态模型'), h('div', { class: 'card__title' }, 'Draft / Saved / Active')),
          validation.ok ? h('span', { class: 'badge badge--ok' }, '合法') : h('span', { class: 'badge badge--err' }, `${validation.errors.length} 错误`)
        ]),
        h('div', { class: 'facts' }, [
          fact('Draft', S.store.dirty ? '有未保存修改' : '与当前 Saved 基线一致'),
          fact('Draft desired', intent.main?.enabled ? '启用' : '禁用'),
          fact('Draft 对象', `节点 ${intent.nodes.length} · 路由 ${intent.routes.length} · DNS ${intent.dns_profiles.length} · 本地入口 ${intent.local_proxies.length} · 规则 ${intent.rules.length} · 订阅 ${intent.subscriptions.length}`),
          fact('Draft warnings', `${validation.warnings?.length || 0} warnings · ${validation.errors?.length || 0} errors`),
          fact('Saved revision', S.fmtRevision(S.store.revision)),
          fact('Saved desired', ov.saved_enabled ? '启用' : '禁用'),
          fact('pending_apply', ov.pending_apply ? '是' : '否'),
          fact('Active generation', status.generation || '—'),
          fact('Active digest', status.intent_digest ? status.intent_digest.slice(0, 12) : '—'),
          fact('Active health', healthy ? 'healthy' : (status.generation ? 'unhealthy' : 'stopped')),
          fact('上次 Apply', lastApply ? `${ui.applyTime(lastApply)} ${lastResult?.ok ? '✓' : '✗'}` : '—')
        ]),
        h('div', { class: 'u-mt-10' }, ui.applyRecord(status)),
        h('p', { class: 'muted' }, validation.errors.length || validation.warnings.length
          ? '这里只显示问题数量；完整 Validation、Probe、报告与日志统一位于“诊断”。'
          : '当前 Draft 校验通过。完整测试与日志统一位于“诊断”。')
      ]);

      function fact(label, value) {
        return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value));
      }

      if (!isCurrent()) return;
      root.append(...[
        ui.viewHead('总览', 'Draft / Saved / Active 摘要与少量恢复操作', [
          h('button', { class: 'btn', onclick: () => S.router?.('diagnostics') }, '打开诊断'),
          h('button', { class: 'btn', onclick: () => S.router?.('system') }, '系统事实')
        ]),
        externalNotice, pipeline, summary
      ].filter(Boolean));
    }
  };

  S.views = S.views || {};
  S.views.overview = view;
})();
