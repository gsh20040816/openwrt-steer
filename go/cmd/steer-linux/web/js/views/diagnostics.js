/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 诊断：直连 / 代理 / 下载测速的详细报告 + 平台语义说明。 */
'use strict';
(function () {
  const S = window.S;
  const { h, fmtDuration, fmtReport } = S;
  const ui = S.ui;

  function probeCard(kind, title, desc) {
    const out = h('div', { class: 'test-slot' }, h('span', { class: 'muted' }, '未测试'));
    const run = h('button', { class: 'btn', onclick: async () => {
      run.disabled = true;
      out.replaceChildren(h('span', { class: 'muted spinning' }, '测试中…'));
      try {
        const report = await S.api.probe(kind);
        const r = fmtReport(report, kind === 'speedtest');
        const raw = report.results?.[0] || {};
        out.replaceChildren(
          h('div', { class: `test-result ${r.ok ? 'is-ok' : 'is-err'}` }, h('strong', {}, r.label), h('small', {}, r.detail)),
          h('div', { class: 'facts u-mt-10' }, [
            fact('URL', raw.url || '—'),
            fact('首字节', fmtDuration(raw.first_byte_milliseconds)),
            fact('连接', fmtDuration(raw.connect_milliseconds)),
            fact('TLS', fmtDuration(raw.tls_milliseconds)),
            fact('HTTP', String(raw.status ?? '—')),
            fact('尝试次数', String(raw.attempts ?? '—')),
            fact('下载字节', raw.downloaded_bytes ? (raw.downloaded_bytes / 1e6).toFixed(1) + ' MB' : '—')
          ])
        );
      } catch (e) {
        out.replaceChildren(h('div', { class: 'test-result is-err' }, h('strong', {}, '失败'), h('small', {}, '详细原因请查看下方诊断日志')));
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

  function fact(label, value) { return h('div', { class: 'fact' }, h('dt', {}, label), h('dd', {}, value)); }

  const view = {
    name: 'diagnostics',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      let logs = { output: '' };
      try { logs = await S.api.logs(); } catch (error) { logs = { output: `日志读取失败：${error.message}` }; }
      if (!isCurrent()) return;
      root.append(
        ui.viewHead('诊断', '探测经当前运行规则执行；裸节点/路由测速在“节点”与“路由”页内进行'),
        h('div', { class: 'grid-3' }, [
          probeCard('direct', '直连探测', '请求直连 URL，验证 Direct 路径'),
          probeCard('proxy', '代理探测', '请求代理 URL，验证当前代理路径'),
          probeCard('speedtest', '下载测速', '经代理下载测速 URL')
        ]),
        h('section', { class: 'card card--edge edge--route' }, [
          h('span', { class: 'eyebrow' }, '平台语义'),
          h('p', { class: 'muted' }, '裸节点与路由测速使用独立临时核心，并显式绕过当前 Steer 数据面；VM/Docker 公网流量由主 TUN 数据面接管。三个测试 URL 均为必填标量，没有隐藏默认值。'),
          h('div', { class: 'row-actions row-actions--wrap' }, [
            h('button', { class: 'btn btn--sm', onclick: () => S.router('nodes') }, '前往节点测速 →'),
            h('button', { class: 'btn btn--sm', onclick: () => S.router('routes') }, '前往路由链测速 →')
          ])
        ]),
        h('section', { class: 'card' }, [
          h('div', { class: 'card__head' }, h('div', {}, h('span', { class: 'eyebrow' }, 'journalctl'), h('div', { class: 'card__title' }, '最近日志'))),
          h('pre', { class: 'mono command-block diagnostics-log' }, logs.output || '当前没有日志输出')
        ])
      );
    }
  };

  S.views = S.views || {};
  S.views.diagnostics = view;
})();
