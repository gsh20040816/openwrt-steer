/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 共享小工具：DOM 构造、转义、图标、格式化。 */
'use strict';
(function () {
  const S = (window.S = window.S || {});

  const esc = (value) => String(value ?? '').replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
  ));

  /* 轻量 DOM 构造器：h('div', {class, onclick, ...}, child…) */
  function h(tag, attrs = {}, ...children) {
    const el = document.createElement(tag);
    for (const [key, value] of Object.entries(attrs)) {
      if (value == null || value === false) continue;
      if (key === 'class') el.className = value;
      else if (key === 'dataset') Object.assign(el.dataset, value);
      else if (key.startsWith('on')) el.addEventListener(key.slice(2), value);
      else if (key === 'html') el.innerHTML = value; /* 仅用于受信任的内联 SVG */
      else el.setAttribute(key, value === true ? '' : String(value));
    }
    for (const child of children.flat(Infinity)) {
      if (child == null || child === false) continue;
      el.append(child instanceof Node ? child : document.createTextNode(String(child)));
    }
    return el;
  }

  const ICONS = {
    gauge: '<circle cx="12" cy="12" r="8.5"/><path d="M12 12 16.4 7.6"/><path d="M12 14.4a2.4 2.4 0 1 0 0-4.8"/>',
    server: '<rect x="3" y="4.5" width="18" height="6.5" rx="1.6"/><rect x="3" y="13" width="18" height="6.5" rx="1.6"/><path d="M7 7.75h.01M7 16.25h.01"/>',
    route: '<circle cx="6" cy="6" r="2.3"/><circle cx="18" cy="18" r="2.3"/><path d="M8.3 6h3.9a3.8 3.8 0 0 1 3.8 3.8v4.4a3.8 3.8 0 0 0 3.8 3.8h-3.9" transform="translate(2 -2)"/>',
    globe: '<circle cx="12" cy="12" r="8.5"/><path d="M3.5 12h17M12 3.5c2.4 2.3 3.6 5.2 3.6 8.5s-1.2 6.2-3.6 8.5c-2.4-2.3-3.6-5.2-3.6-8.5s1.2-6.2 3.6-8.5z"/>',
    plug: '<path d="M9 7.5V3.5m6 4V3.5M7 7.5h10v4.2a5 5 0 0 1-10 0z"/><path d="M12 16.7v3.8"/>',
    list: '<path d="M8.5 6h11M8.5 12h11M8.5 18h11"/><circle cx="4.2" cy="6" r="1"/><circle cx="4.2" cy="12" r="1"/><circle cx="4.2" cy="18" r="1"/>',
    refresh: '<path d="M19.6 11.5a7.6 7.6 0 1 0-2.2 5.4"/><path d="M19.8 4.2v7.3h-7.3"/>',
    activity: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
    sliders: '<path d="M4 8h10M18 8h2M4 16h4M12 16h8"/><circle cx="16" cy="8" r="2"/><circle cx="10" cy="16" r="2"/>',
    braces: '<path d="M8.5 4.5c-2 0-3 .9-3 2.6v2.3c0 1.6-.9 2-2 2.6 1.1.6 2 1 2 2.6v2.3c0 1.7 1 2.6 3 2.6M15.5 4.5c2 0 3 .9 3 2.6v2.3c0 1.6.9 2 2 2.6-1.1.6-2 1-2 2.6v2.3c0 1.7-1 2.6-3 2.6"/>'
  };

  const icon = (name, size = 15) => h('span', {
    class: 'icon', 'aria-hidden': 'true',
    html: `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">${ICONS[name] || ''}</svg>`
  });

  const asList = (value) => (value == null ? [] : (Array.isArray(value) ? value : [value]));

  const fmtDuration = (ms) => (ms == null ? '—' : (ms >= 1000 ? (ms / 1000).toFixed(1) + ' s' : Math.round(ms) + ' ms'));
  const fmtTime = (iso) => (iso ? new Date(iso).toLocaleString('zh-CN', { hour12: false }) : '—');
  const fmtRevision = (value) => {
    const revision = String(value || '').replace(/^"|"$/g, '');
    return revision.length > 22 ? `${revision.slice(0, 15)}…` : (revision || '—');
  };

  /* 测试报告 → { ok, label, detail }（人话化） */
  function fmtReport(report, download) {
    const results = report?.results || [];
    const okResults = results.filter((r) => r.ok === true);
    if (!okResults.length) {
      return {
        ok: false, label: '失败',
        detail: '详细原因请查看诊断日志'
      };
    }
    if (download) {
      const measured = okResults
        .filter((r) => r.downloaded_bytes > 0 && r.download_milliseconds > 0)
        .map((r) => ({ r, mbps: r.downloaded_bytes * 8 / r.download_milliseconds / 1000 }))
        .sort((a, b) => b.mbps - a.mbps);
      if (!measured.length) return { ok: false, label: '失败', detail: '没有下载测量结果' };
      const { r, mbps } = measured[0];
      return { ok: true, label: `${mbps.toFixed(1)} Mbps`, detail: `${(r.downloaded_bytes / 1e6).toFixed(1)} MB in ${fmtDuration(r.download_milliseconds)} · HTTP ${r.status} · ${r.attempts} attempt(s)` };
    }
    const r = okResults[0];
    const latency = r.first_byte_milliseconds ?? r.tls_milliseconds ?? r.connect_milliseconds ?? 0;
    return {
      ok: true, label: fmtDuration(latency),
      detail: `Connect ${fmtDuration(r.connect_milliseconds)} · TLS ${fmtDuration(r.tls_milliseconds)} · HTTP ${r.status} · ${r.attempts} attempt(s)`
    };
  }

  function probeReportIsStale(report, diagnostics, draftDirty = false) {
    if (!report) return false;
    if (report.scope === 'overview') {
      return !report.saved_digest || report.saved_digest !== diagnostics?.saved_digest ||
        (report.active_generation || '') !== (diagnostics?.active_generation || '') ||
        (report.active_digest || '') !== (diagnostics?.active_digest || '');
    }
    return draftDirty || !report.saved_digest || report.saved_digest !== diagnostics?.saved_digest;
  }

  function safeProbeError(report) {
    const error = String(report?.error || report?.results?.find((result) => result?.error)?.error || '');
    return {
      'probe timed out': '连接超时',
      'probe was cancelled': '测试已取消',
      'TLS verification failed': 'TLS 校验失败',
      'probe connection was refused': '连接被拒绝',
      'probe target could not be resolved': '目标无法解析'
    }[error] || '请查看诊断日志';
  }

  function fmtLatestProbe(report, diagnostics, draftDirty = false) {
    if (!report) return { text: '尚未测试', ok: null, stale: false };
    const stale = probeReportIsStale(report, diagnostics, draftDirty);
    const download = report.kind === 'download' || report.kind === 'speedtest';
    const result = fmtReport(report, download);
    const metric = result.ok ? result.label : safeProbeError(report);
    return {
      text: `上次 ${fmtTime(report.tested_at)} · ${stale ? '已过期 · ' : ''}${result.ok ? '成功' : '失败'} · ${metric}`,
      ok: result.ok,
      stale
    };
  }

  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const debounce = (fn, ms) => {
    let timer;
    return (...args) => { clearTimeout(timer); timer = setTimeout(() => fn(...args), ms); };
  };
  const uid = (prefix) => `${prefix}-${Math.random().toString(36).slice(2, 8)}`;

  Object.assign(S, {
    esc, h, icon, asList, fmtDuration, fmtTime, fmtRevision, fmtReport,
    probeReportIsStale, fmtLatestProbe, sleep, debounce, uid
  });
})();
