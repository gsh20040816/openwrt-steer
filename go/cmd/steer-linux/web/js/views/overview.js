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
      const active = !!status.generation;
      const savedEnabled = ov.saved_enabled === true;
      const pendingApply = ov.pending_apply === true;
      const externalChange = S.store.hasExternalChange === true;

      const externalNotice = externalChange ? h('section', { class: 'card card--edge edge--err' }, [
        h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '配置冲突'), h('div', { class: 'card__title' }, '服务器配置已变化'))),
        h('p', {}, S.store.dirty
          ? '当前工作副本已保留且不会自动覆盖。请先保存、放弃或在顶部处理配置冲突。'
          : '点击顶部“重新载入”即可更新为服务器上的最新配置。')
      ]) : null;

      const pipeline = h('section', { class: 'card hero', 'data-overview-region': 'execution_model' }, [
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

      const lifecycle = h('section', { class: 'card', 'data-overview-region': 'configuration_lifecycle' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '配置生命周期'), h('div', { class: 'card__title' }, 'Draft、Saved 与 Active')),
          pendingApply || savedEnabled !== active ? h('span', { class: 'badge badge--warn' }, '状态不一致') : h('span', { class: 'badge badge--ok' }, '状态一致')
        ]),
        h('div', { class: 'facts' }, [
          fact('工作副本', S.store.dirty ? '有未保存修改' : '已保存'),
          fact('工作副本开关', intent.main?.enabled ? '启用' : '禁用'),
          fact('已保存配置', savedEnabled ? '启用' : '禁用'),
          fact('等待应用', pendingApply ? '是' : '否'),
          fact('当前运行', active ? (healthy ? '正常' : '异常') : '已停止'),
          fact('Saved / Active', pendingApply || savedEnabled !== active ? '不一致，等待处理' : '一致')
        ])
      ]);

      const scale = h('section', { class: 'card', 'data-overview-region': 'object_scale' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '配置规模'), h('div', { class: 'card__title' }, '当前工作副本'))
        ]),
        h('div', { class: 'facts' }, [
          fact('节点', intent.nodes.length), fact('路由', intent.routes.length),
          fact('DNS Profile', intent.dns_profiles.length), fact('本地入口', intent.local_proxies.length),
          fact('规则', intent.rules.length), fact('订阅', intent.subscriptions.length)
        ])
      ]);

      const validationSummary = h('section', { class: 'card', 'data-overview-region': 'validation_summary' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '校验与警告摘要'), h('div', { class: 'card__title' }, '当前工作副本')),
          validation.ok ? h('span', { class: 'badge badge--ok' }, '合法') : h('span', { class: 'badge badge--err' }, `${validation.errors.length} 错误`)
        ]),
        h('div', { class: 'facts' }, [
          fact('错误', validation.errors?.length || 0),
          fact('警告', validation.warnings?.length || 0),
          fact('警告分组', validation.warning_groups?.length || 0)
        ]),
        warningGroups(validation),
        h('p', { class: 'muted' }, validation.errors.length || validation.warnings.length
          ? '同类警告已按正在使用的对象聚合；详细修复入口位于对应页面。'
          : '当前工作副本校验通过。')
      ]);

      const lastApplySummary = h('section', { class: 'card', 'data-overview-region': 'last_apply_and_actions' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '最近应用与快捷操作'), h('div', { class: 'card__title' }, lastApply ? `${ui.applyTime(lastApply)} · ${lastResult?.ok ? '成功' : '失败'}` : '尚无应用记录')),
          h('div', { class: 'toolbar' }, [
            h('button', { class: 'btn', onclick: ui.onRefreshState }, '刷新'),
            h('button', { class: 'btn', onclick: () => S.router?.('diagnostics') }, '打开诊断'),
            h('button', { class: 'btn', onclick: () => S.router?.('system') }, '系统信息')
          ])
        ]),
        ui.applyRecord(status),
        h('p', { class: 'muted' }, '保存、保存并应用、应用已保存配置和放弃修改由本页顶部全局状态区按当前 Draft 状态提供。')
      ]);

      function fact(label, value) {
        return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value));
      }

      if (!isCurrent()) return;
      root.append(...[
        ui.viewHead('总览', '执行模型、配置生命周期与当前工作副本概览'),
        externalNotice, pipeline, lifecycle, scale, validationSummary, lastApplySummary
      ].filter(Boolean));
    }
  };

  S.views = S.views || {};
  S.views.overview = view;
})();
