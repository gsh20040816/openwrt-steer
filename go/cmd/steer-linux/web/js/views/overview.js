/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 总览：工作副本、已保存配置、运行状态与少量快捷操作。 */
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

  function warningGroupLabel(group) {
    if (group.code === 'INSECURE_TLS' && group.object_type === 'dns_profile') return 'DNS 证书校验已关闭';
    if (group.code === 'INSECURE_TLS') return 'TLS 证书校验已关闭';
    if (group.code === 'SUBSCRIPTION_NODE_STALE') return '订阅已不再提供此节点';
    if (group.code === 'DNS_REJECT_PROJECTION_SKIPPED') return 'DNS 拒绝条件无法在解析前完整执行';
    if (group.code === 'DNS_PROJECTION_EMPTY') return 'DNS 将继续匹配后续规则';
    return group.summary || '配置警告';
  }

  function warningGroupScope(group) {
    return ({ node: '节点', route: '路由', dns_profile: 'DNS Profile', local_proxy: '本地入口', rule: '规则' })[group.object_type] || '对象';
  }

  function warningGroups(validation) {
    const groups = Array.isArray(validation?.warning_groups) ? validation.warning_groups : [];
    if (!groups.length) return null;
    return h('div', { class: 'warning-groups', 'aria-label': '警告摘要' }, groups.map((group) => h('article', { class: 'warning-group' }, [
      h('div', { class: 'warning-group__copy' }, [
        h('strong', {}, warningGroupLabel(group)),
        h('span', {}, `${group.count || 0} 个正在使用的${warningGroupScope(group)}`)
      ]),
      group.destination ? h('button', { class: 'btn btn--sm', onclick: () => S.router?.(group.destination) }, `查看${warningGroupScope(group)}`) : null
    ])));
  }

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
        h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '配置冲突'), h('div', { class: 'card__title' }, '服务器配置已变化'))),
        h('p', {}, S.store.dirty
          ? '当前工作副本已保留且不会自动覆盖。请先保存、放弃或在顶部处理配置冲突。'
          : '点击顶部“重新载入”即可更新为服务器上的最新配置。')
      ]) : null;

      const pipeline = h('section', { class: 'card hero' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '执行模型'), h('div', { class: 'card__title' }, '流量如何被转向')),
          h('span', { class: 'badge badge--match' }, '首条匹配 · 严格有序')
        ]),
        h('div', { class: 'pipeline' }, [
          step('match', intent.rules.length, '匹配规则', '首条命中即停'),
          arrow(),
          step('dns', intent.dns_profiles.length, 'DNS Profile', '独立解析路径'),
          arrow(),
          step('route', intent.routes.length, '路由', '直连 / 拒绝 / 节点链'),
          arrow(),
          step('net', null, '网络出口', '前置节点链路')
        ])
      ]);

      const summary = h('section', { class: 'card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '配置状态'), h('div', { class: 'card__title' }, '工作副本、已保存配置与运行状态')),
          validation.ok ? h('span', { class: 'badge badge--ok' }, '合法') : h('span', { class: 'badge badge--err' }, `${validation.errors.length} 错误`)
        ]),
        h('div', { class: 'facts' }, [
          fact('工作副本', S.store.dirty ? '有未保存修改' : '已保存'),
          fact('配置开关', intent.main?.enabled ? '启用' : '禁用'),
          fact('配置规模', `节点 ${intent.nodes.length} · 路由 ${intent.routes.length} · DNS ${intent.dns_profiles.length} · 本地入口 ${intent.local_proxies.length} · 规则 ${intent.rules.length} · 订阅 ${intent.subscriptions.length}`),
          fact('配置校验', `${validation.warnings?.length || 0} 项警告 · ${validation.errors?.length || 0} 项错误`),
          fact('已保存开关', ov.saved_enabled ? '启用' : '禁用'),
          fact('待应用更改', ov.pending_apply ? '有' : '无'),
          fact('运行状态', healthy ? '正常运行' : (status.generation ? '运行异常' : '已停止')),
          fact('上次应用', lastApply ? `${ui.applyTime(lastApply)} ${lastResult?.ok ? '✓' : '✗'}` : '—')
        ]),
        h('div', { class: 'u-mt-10' }, ui.applyRecord(status)),
        warningGroups(validation),
        h('p', { class: 'muted' }, validation.errors.length || validation.warnings.length
          ? '可在“诊断”页面查看详细问题、连通性报告与系统日志。'
          : '当前工作副本校验通过。可在“诊断”页面查看连通性报告与系统日志。')
      ]);

      function fact(label, value) {
        return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value));
      }

      if (!isCurrent()) return;
      root.append(...[
        ui.viewHead('总览', '工作副本与运行状态概览', [
          h('button', { class: 'btn', onclick: () => S.router?.('diagnostics') }, '打开诊断'),
          h('button', { class: 'btn', onclick: () => S.router?.('system') }, '系统信息')
        ]),
        externalNotice, pipeline, summary
      ].filter(Boolean));
    }
  };

  S.views = S.views || {};
  S.views.overview = view;
})();
