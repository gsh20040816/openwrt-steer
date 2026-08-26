/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 诊断：共享的 Probe 历史、Validation、Apply 与相关服务日志。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtDuration, fmtReport, fmtTime } = S;
  const ui = S.ui;

  const overviewBoundary = '按当前 Active 规则访问配置的测试目标。成功只表示该 URL 当时可达，不证明具体 outbound、DNS resolver 或 DNS 无泄漏。';

  function fact(label, value) { return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value)); }

  function reportIsStale(report, diagnostics) {
    if (report.scope === 'overview') {
      return !report.active_generation || !report.active_digest ||
        report.active_generation !== diagnostics.active_generation || report.active_digest !== diagnostics.active_digest;
    }
    return S.store.dirty || !report.saved_digest || report.saved_digest !== diagnostics.saved_digest;
  }

  function reportView(report, diagnostics) {
    const stale = reportIsStale(report, diagnostics);
    const result = report.results?.[0] || {};
    const rate = result.downloaded_bytes > 0 && result.download_milliseconds > 0
      ? (result.downloaded_bytes * 8 / result.download_milliseconds / 1000).toFixed(1) + ' Mbps'
      : '—';
    const target = report.scope === 'overview' ? 'Overview' : `${report.scope}/${report.object_id || '—'}`;
    return h('article', { class: 'card card--compact' }, [
      h('div', { class: 'card__head' }, [
        h('div', {}, h('strong', {}, target), h('span', { class: 'mono muted' }, ` · ${report.kind}`)),
        h('span', { class: `badge ${stale ? 'badge--warn' : (report.ok ? 'badge--ok' : 'badge--err')}` }, stale ? '已过期' : (report.ok ? '成功' : '失败'))
      ]),
      h('div', { class: 'facts' }, [
        fact('tested_at', report.tested_at ? fmtTime(report.tested_at) : '—'),
        fact('URL', result.url || '—'),
        fact('尝试', String(result.attempts ?? '—')),
        fact('连接', fmtDuration(result.connect_milliseconds)),
        fact('TLS', fmtDuration(result.tls_milliseconds)),
        fact('TTFB', fmtDuration(result.first_byte_milliseconds)),
        fact('HTTP', String(result.status ?? '—')),
        fact('下载', result.downloaded_bytes ? `${result.downloaded_bytes} bytes / ${fmtDuration(result.download_milliseconds)}` : '—'),
        fact('速率', rate)
      ]),
      report.error || result.error ? h('p', { class: 'alert alert--err' }, report.error || result.error) : null
    ]);
  }

  function probeCard(kind, title, description) {
    const out = h('div', { class: 'test-slot' }, h('span', { class: 'muted' }, '未测试'));
    const run = h('button', { class: 'btn', onclick: async () => {
      run.disabled = true;
      out.replaceChildren(h('span', { class: 'muted spinning' }, '测试中…'));
      try {
        const report = await S.api.probe(kind);
        const summary = fmtReport(report, kind === 'speedtest');
        const result = report.results?.[0] || {};
        out.replaceChildren(
          h('div', { class: `test-result ${summary.ok ? 'is-ok' : 'is-err'}` }, h('strong', {}, summary.label), h('small', {}, summary.detail)),
          h('div', { class: 'facts u-mt-10' }, [
            fact('tested_at', report.tested_at ? fmtTime(report.tested_at) : '—'),
            fact('URL', result.url || '—'), fact('连接', fmtDuration(result.connect_milliseconds)),
            fact('TLS', fmtDuration(result.tls_milliseconds)), fact('TTFB', fmtDuration(result.first_byte_milliseconds)),
            fact('HTTP', String(result.status ?? '—')), fact('尝试次数', String(result.attempts ?? '—'))
          ])
        );
      } catch (error) {
        out.replaceChildren(h('div', { class: 'test-result is-err' }, h('strong', {}, '失败'), h('small', {}, '已保存安全错误摘要；刷新下方报告查看')));
      }
      run.disabled = false;
    } }, '运行测试');
    return h('div', { class: 'test-card' }, [h('span', { class: 'eyebrow' }, title), h('p', {}, description), run, out]);
  }

  const view = {
    name: 'diagnostics',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      let logs = { output: '' }, diagnostics = { reports: [], warnings: [] }, validation = { errors: [], warnings: [] };
      [logs, diagnostics, validation] = await Promise.all([
        S.api.logs().catch((error) => ({ output: `日志刷新失败：${error.message}` })),
        S.api.diagnostics().catch((error) => ({ reports: [], warnings: [`报告刷新失败：${error.message}`] })),
        S.api.validate(S.store.intent).catch((error) => ({ errors: [{ code: 'VALIDATION_UNAVAILABLE', message: error.message }], warnings: [] }))
      ]);
      const status = S.store.overview?.status || {};
      if (!isCurrent()) return;
      const refresh = h('button', { class: 'btn', onclick: async () => {
        refresh.disabled = true;
        try { await S.store.refreshOverview(); } finally { if (isCurrent()) view.render(root); }
      } }, '刷新诊断');
      root.append(
        ui.viewHead('诊断', overviewBoundary, [refresh]),
        h('div', { class: 'grid-3' }, [
          probeCard('direct', '直连目标', '按当前 Active 规则访问配置的直连测试目标'),
          probeCard('proxy', '代理目标', '按当前 Active 规则访问配置的代理测试目标'),
          probeCard('speedtest', '下载目标', '按当前 Active 规则访问配置的下载测试目标')
        ]),
        h('section', { class: 'card card--edge edge--route' }, [
          h('span', { class: 'eyebrow' }, '能力边界'),
          h('p', { class: 'muted' }, overviewBoundary),
          h('p', { class: 'muted' }, '节点与路由测试使用独立临时核心验证所选链路访问测试目标，不代表当前 Active 规则命中了该节点或路由。')
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'Reports'), h('div', { class: 'card__title' }, '最近 Overview / Node / Route 报告'))),
          ...(diagnostics.warnings || []).map((warning) => h('p', { class: 'alert' }, warning)),
          (diagnostics.reports || []).length ? h('div', { class: 'stack' }, diagnostics.reports.map((report) => reportView(report, diagnostics))) : h('p', { class: 'muted' }, '尚无已保存 Probe 报告')
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'Validation'), h('div', { class: 'card__title' }, '当前 Draft 校验'))),
          (validation.errors || []).length || (validation.warnings || []).length
            ? h('div', {}, [ui.issueList(validation.errors || [], ui.jumpToObject), ui.issueList(validation.warnings || [], ui.jumpToObject, true)])
            : h('p', { class: 'muted' }, '当前 Draft 校验通过')
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'Apply'), h('div', { class: 'card__title' }, '最近 Apply 结果'))),
          h('div', { class: 'facts' }, [fact('当前 generation', status.generation || '—'), fact('当前 Intent digest', status.intent_digest ? status.intent_digest.slice(0, 12) : '—'), fact('运行健康', status.healthy ? '是' : '否')]),
          ui.applyRecord(status)
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'journalctl'), h('div', { class: 'card__title' }, 'steer / steer-web / steer-subscription'))),
          h('pre', { class: 'mono command-block diagnostics-log' }, logs.output || '当前没有日志输出')
        ])
      );
    }
  };

  S.views = S.views || {};
  S.views.diagnostics = view;
})();
