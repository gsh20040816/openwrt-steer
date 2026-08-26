/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 总览：执行模型流水线 + 三张测试卡 + 校验摘要 + 快照。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtReport } = S;
  const ui = S.ui;

  function step(kind, num, title, sub) {
    return h('div', { class: `step step--${kind}` }, [
      num != null ? h('span', { class: 'step__num' }, num) : null,
      h('strong', {}, title),
      h('small', {}, sub)
    ]);
  }
  const arrow = () => h('span', { class: 'arrow', 'aria-hidden': 'true' }, '→');

  function testCard(kind, title, desc) {
    const out = h('div', { class: 'test-slot' }, h('span', { class: 'muted' }, '未测试'));
    const run = h('button', { class: 'btn', onclick: async () => {
      run.disabled = true;
      out.replaceChildren(h('span', { class: 'muted spinning' }, '测试中…'));
      try {
        const report = await S.api.probe(kind);
        const r = fmtReport(report, kind === 'speedtest');
        out.replaceChildren(h('div', { class: `test-result ${r.ok ? 'is-ok' : 'is-err'}` }, h('strong', {}, r.label), h('small', {}, r.detail)));
      } catch (e) {
        out.replaceChildren(h('div', { class: 'test-result is-err' }, h('strong', {}, '失败'), h('small', {}, '详细原因请查看诊断日志')));
      }
      run.disabled = false;
    } }, '运行测试');
    return h('div', { class: 'test-card' }, [
      h('span', { class: 'eyebrow' }, title),
      h('p', {}, desc),
      run,
      out
    ]);
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

      const tests = h('section', { class: 'card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '连通性'), h('div', { class: 'card__title' }, '手动探测')),
          h('span', { class: 'muted' }, '按当前 Active 规则访问 · 只证明目标当时可达')
        ]),
        h('div', { class: 'grid-3' }, [
          testCard('direct', '直连目标', '按当前 Active 规则访问配置的直连测试目标；不证明具体 outbound 或 DNS resolver'),
          testCard('proxy', '代理目标', '按当前 Active 规则访问配置的代理测试目标；不证明具体 outbound 或 DNS resolver'),
          testCard('speedtest', '下载目标', '按当前 Active 规则访问配置的下载测试目标；不证明具体 outbound 或 DNS resolver')
        ])
      ]);

      const summary = h('section', { class: 'card' }, [
        h('div', { class: 'card__head' }, [
          h('div', {}, h('span', { class: 'eyebrow' }, '配置'), h('div', { class: 'card__title' }, '已保存配置')),
          validation.ok ? h('span', { class: 'badge badge--ok' }, '合法') : h('span', { class: 'badge badge--err' }, `${validation.errors.length} 错误`)
        ]),
        h('div', { class: 'facts' }, [
          fact('修订', S.fmtRevision(S.store.revision)),
          fact('节点', String(intent.nodes.length)),
          fact('订阅', String(intent.subscriptions.length)),
          fact('DNS Profile', String(intent.dns_profiles.length)),
          fact('本地代理', String(intent.local_proxies.length)),
          fact('运行 generation', status.generation || '—'),
          fact('运行 digest', status.intent_digest ? status.intent_digest.slice(0, 12) : '—'),
          fact('系统服务', healthy ? 'active' : (status.generation ? '不健康' : 'stopped')),
          fact('上次 Apply', lastApply ? `${ui.applyTime(lastApply)} ${lastResult?.ok ? '✓' : '✗'}` : '—')
        ]),
        h('div', { class: 'u-mt-10' }, ui.applyRecord(status)),
        validation.errors.length || validation.warnings.length ? h('div', {}, [
          ui.issueList(validation.errors, ui.jumpToObject),
          ui.issueList(validation.warnings, ui.jumpToObject, true)
        ]) : h('p', { class: 'muted' }, '校验通过。切换运行态失败会保留现场，不会伪装成功。')
      ]);

      function fact(label, value) {
        return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value));
      }

      if (!isCurrent()) return;
      root.append(
        ui.viewHead('总览', 'systemd Linux 透明代理控制面 · 主机及 VM/Docker 公网流量统一进入 Steer'),
        pipeline, tests, summary
      );
    }
  };

  S.views = S.views || {};
  S.views.overview = view;
})();
