/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 诊断：连通性测试、配置校验、应用结果与相关服务日志。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtDuration, fmtReport, fmtTime } = S;
  const ui = S.ui;

  const overviewBoundary = '使用当前运行配置访问测试地址；成功仅表示该地址在测试时可达。';

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
    const scopeLabel = { overview: '总览', node: '节点', nodes: '节点', route: '路由', routes: '路由' }[report.scope] || '测试对象';
    const target = report.scope === 'overview' ? scopeLabel : scopeLabel;
    const kind = { direct: '直连测试', proxy: '代理测试', speedtest: '下载测试', connect: '连接测试', download: '下载测试' }[report.kind] || '连通性测试';
    return h('article', { class: 'card card--compact' }, [
      h('div', { class: 'card__head' }, [
        h('div', {}, h('strong', {}, target), h('span', { class: 'muted' }, ` · ${kind}`)),
        h('span', { class: `badge ${stale ? 'badge--warn' : (report.ok ? 'badge--ok' : 'badge--err')}` }, stale ? '已过期' : (report.ok ? '成功' : '失败'))
      ]),
      h('div', { class: 'facts' }, [
        fact('测试时间', report.tested_at ? fmtTime(report.tested_at) : '—'),
        fact('URL', result.url || '—'),
        fact('尝试', String(result.attempts ?? '—')),
        fact('连接', fmtDuration(result.connect_milliseconds)),
        fact('TLS', fmtDuration(result.tls_milliseconds)),
        fact('TTFB', fmtDuration(result.first_byte_milliseconds)),
        fact('HTTP', String(result.status ?? '—')),
        fact('下载', result.downloaded_bytes ? `${result.downloaded_bytes} 字节 / ${fmtDuration(result.download_milliseconds)}` : '—'),
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
            fact('测试时间', report.tested_at ? fmtTime(report.tested_at) : '—'),
            fact('URL', result.url || '—'), fact('连接', fmtDuration(result.connect_milliseconds)),
            fact('TLS', fmtDuration(result.tls_milliseconds)), fact('TTFB', fmtDuration(result.first_byte_milliseconds)),
            fact('HTTP', String(result.status ?? '—')), fact('尝试次数', String(result.attempts ?? '—'))
          ])
        );
      } catch (error) {
        out.replaceChildren(h('div', { class: 'test-result is-err' }, h('strong', {}, '失败'), h('small', {}, '测试未成功，详情请查看下方报告')));
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
      const dnsCapture = diagnostics.dns_capture || {};
      if (!isCurrent()) return;
      const refresh = h('button', { class: 'btn', onclick: async () => {
        refresh.disabled = true;
        try { await S.store.refreshOverview(); } finally { if (isCurrent()) view.render(root); }
      } }, '刷新诊断');
      root.append(
        ui.viewHead('诊断', overviewBoundary, [refresh]),
        h('div', { class: 'grid-3' }, [
          probeCard('direct', '直连目标', '测试直连地址是否可访问'),
          probeCard('proxy', '代理目标', '测试代理地址是否可访问'),
          probeCard('speedtest', '下载目标', '测试代理下载速度')
        ]),
        h('section', { class: 'card card--edge edge--dns' }, [
          h('div', { class: 'card__head' }, [
            h('div', {}, h('span', { class: 'eyebrow' }, 'DNS'), h('div', { class: 'card__title' }, '系统 DNS 接管检查')),
            h('span', { class: `badge ${dnsCapture.configured ? 'badge--ok' : 'badge--warn'}` }, dnsCapture.configured ? '已配置' : '未确认')
          ]),
          h('div', { class: 'facts' }, [
            fact('结果', dnsCapture.detail === 'the published Active generation contains the expected port-53 capture artifacts'
              ? '系统 DNS 接管已配置'
              : (dnsCapture.detail || '尚未检查'))
          ])
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '测试报告'), h('div', { class: 'card__title' }, '最近连通性报告'))),
          ...(diagnostics.warnings || []).map((warning) => h('p', { class: 'alert' }, warning)),
          (diagnostics.reports || []).length ? h('div', { class: 'stack' }, diagnostics.reports.map((report) => reportView(report, diagnostics))) : h('p', { class: 'muted' }, '尚无已保存测试报告')
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '配置校验'), h('div', { class: 'card__title' }, '当前工作副本'))),
          (validation.errors || []).length || (validation.warnings || []).length
            ? h('div', {}, [ui.issueList(validation.errors || [], ui.jumpToObject), ui.issueList(validation.warnings || [], ui.jumpToObject, true)])
            : h('p', { class: 'muted' }, '当前工作副本校验通过')
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '应用记录'), h('div', { class: 'card__title' }, '最近应用结果'))),
          h('div', { class: 'facts' }, [fact('运行状态', status.healthy ? '正常' : (status.generation ? '异常' : '已停止'))]),
          ui.applyRecord(status)
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, '系统日志'), h('div', { class: 'card__title' }, 'Steer 服务日志'))),
          h('pre', { class: 'mono command-block diagnostics-log' }, logs.output || '当前没有日志输出')
        ])
      );
    }
  };

  S.views = S.views || {};
  S.views.diagnostics = view;
})();
