/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 外壳与共享组件：侧栏、状态条、通知、抽屉、对话框、表单件、Match 编辑器、问题列表。 */
'use strict';
(function () {
  const S = window.S;
  const { h, icon, asList } = S;

  const renderTokens = new WeakMap();
  const routeTokens = new WeakMap();
  let routeSequence = 0;

  function beginRender(root) {
    const token = {};
    renderTokens.set(root, token);
    root.replaceChildren();
    return () => renderTokens.get(root) === token;
  }

  function beginRoute(root) {
    const token = ++routeSequence;
    routeTokens.set(root, token);
    root.replaceChildren();
    return token;
  }

  function isCurrentRoute(root, token) {
    return routeTokens.get(root) === token;
  }

  const NAV = [
    { label: '总览', items: [['overview', 'gauge', '总览']] },
    {
      label: '配置', items: [
        ['nodes', 'server', '节点'],
        ['routes', 'route', '路由'],
        ['dns', 'globe', 'DNS Profile'],
        ['proxies', 'plug', '本地代理'],
        ['rules', 'list', '规则']
      ]
    },
    {
      label: '运维', items: [
        ['subscriptions', 'refresh', '订阅'],
        ['diagnostics', 'activity', '诊断'],
        ['system', 'sliders', '系统'],
        ['config', 'braces', '配置 · 高级']
      ]
    }
  ];

  /* ---------- 侧栏 ---------- */
  function renderShell(router) {
    const side = document.querySelector('#side');
    const brand = h('div', { class: 'brand' }, [
      h('strong', {}, 'steer'),
      h('span', { class: 'brand__sub' }, 'linux · loopback console')
    ]);
    const nav = h('nav', {});
    for (const group of NAV) {
      nav.append(h('div', { class: 'nav-group__label' }, group.label));
      for (const [view, ic, label] of group.items) {
        nav.append(h('button', {
          class: 'nav-item', dataset: { view },
          onclick: () => router(view)
        }, icon(ic), h('span', {}, label)));
      }
    }
    const foot = h('div', { class: 'side-foot' }, [
      h('button', {
        class: 'btn btn--sm',
        onclick: () => S.auth.logout()
      }, '退出登录'),
      h('div', { class: 'side-foot__note' }, 'Bearer 认证 · 令牌仅保存在当前标签页\nloopback only · 不对公网监听')
    ]);
    side.append(brand, nav, foot);
  }

  /* ---------- 状态条 ---------- */
  async function onValidate() {
    const [v, geosite, geoip] = await Promise.all([
      S.api.validate(S.store.intent), S.api.geodata('geosite'), S.api.geodata('geoip')
    ]);
    const geoErrors = S.validateGeoCategories(S.store.intent, { geosite, geoip }).map((message) => ({
      code: 'UNKNOWN_GEO_CATEGORY', object_type: 'rule', message
    }));
    v.errors.push(...geoErrors);
    v.ok = v.errors.length === 0;
    if (!v.errors.length && !v.warnings.length) { toast('校验通过 · 0 错误 · 0 警告', 'ok'); return; }
    dialog({
      title: '配置校验',
      body: h('div', {}, [
        h('p', { class: 'muted' }, `${v.errors.length} 错误 · ${v.warnings.length} 警告 · 校验的是当前工作副本`),
        issueList(v.errors, jumpToObject),
        issueList(v.warnings, jumpToObject, true)
      ]),
      actions: [['关闭', null]]
    });
  }

  function jumpToObject(issue) {
    const target = { node: 'nodes', route: 'routes', rule: 'rules', dns_profile: 'dns', local_proxy: 'proxies', subscription: 'subscriptions' }[issue.object_type];
    if (target && S.router) S.router(target);
  }

  async function onSave(apply) {
    try {
      const res = await S.store.save(apply);
      if (res.ok) {
        if (apply && !res.res.applied) {
          toast(`已保存，但 Apply 失败：${res.res.apply_result?.error || res.res.error?.message || '运行态未切换'}`, 'err');
        } else {
          toast(apply ? `已保存并 Apply · generation ${res.res.apply_result?.generation || '已切换'}` : `已保存 · 修订 ${S.fmtRevision(S.store.revision)}`, 'ok');
        }
      } else if (res.conflict) {
        conflictDialog(res.conflict);
      }
    } catch (error) {
      toast(`保存失败：${error.message}`, 'err');
    }
  }

  async function forceSave() {
    try {
      const res = await S.store.save(false, true);
      if (res.ok) toast(`已覆盖保存 · 修订 ${S.fmtRevision(S.store.revision)}`, 'ok');
    } catch (error) {
      toast(`覆盖保存失败：${error.message}`, 'err');
    }
  }

  function renderStatusStrip() {
    const strip = document.querySelector('#strip');
    strip.replaceChildren();
    const ov = S.store.overview || {};
    const status = ov.status || {};
    const lastApply = status.last_apply || null;
    const lastResult = lastApply?.result || lastApply;
    const healthy = !!status.healthy;
    const dirty = S.store.dirty;

    strip.append(
      h('div', { class: 'strip__group' }, [
        h('span', { class: `health-dot ${healthy ? 'is-ok' : 'is-err'}`, title: healthy ? '运行健康' : '不健康' }),
        h('div', { class: 'strip__fact' }, h('span', { class: 'strip__fact-label' }, '运行状态'), h('span', { class: 'strip__fact-value' }, healthy ? 'healthy' : 'unhealthy')),
        h('div', { class: 'strip__fact' }, h('span', { class: 'strip__fact-label' }, 'systemd'), h('span', { class: 'strip__fact-value' }, healthy ? 'active' : 'inactive')),
        h('div', { class: 'strip__fact' }, h('span', { class: 'strip__fact-label' }, 'generation'), h('span', { class: 'strip__fact-value' }, status.generation || lastResult?.generation || '—')),
        h('div', { class: 'strip__fact' }, h('span', { class: 'strip__fact-label' }, '修订'), h('span', { class: 'strip__fact-value', title: S.store.revision }, S.fmtRevision(S.store.revision))),
        h('div', { class: 'strip__fact' }, h('span', { class: 'strip__fact-label' }, '上次 Apply'), h('span', { class: 'strip__fact-value' }, lastApply ? `#${lastApply.sequence} ${lastResult?.ok ? '✓' : '✗'}` : '—')),
        dirty ? h('span', { class: 'badge badge--warn', title: '工作副本有未保存修改' }, '工作副本已修改') : null
      ]),
      h('div', { class: 'strip__actions' }, [
        h('button', { class: 'btn', onclick: onValidate }, '校验'),
        h('button', { class: 'btn', onclick: () => onSave(false), disabled: !dirty }, '保存'),
        h('button', { class: 'btn btn--primary', onclick: () => onSave(true), disabled: !dirty }, '保存并 Apply')
      ])
    );
  }

  /* ---------- 通知 ---------- */
  function toast(message, kind = 'info') {
    const box = document.querySelector('#toasts');
    const el = h('div', { class: `toast toast--${kind}` }, h('span', {}, message));
    box.append(el);
    setTimeout(() => {
      el.classList.add('is-leaving');
      setTimeout(() => el.remove(), 320);
    }, 4200);
  }

  /* ---------- 对话框 ---------- */
  function dialog({ title, body, actions = [], width }) {
    const overlay = h('div', { class: 'dialog-overlay' });
    const close = () => {
      overlay.remove();
      document.removeEventListener('keydown', onEsc);
    };
    const onEsc = (e) => { if (e.key === 'Escape') close(); };
    overlay.append(h('div', { class: `dialog ${width ? 'dialog--wide' : ''}` }, [
      h('div', { class: 'dialog__head' }, h('strong', {}, title), h('button', { class: 'dialog__close', onclick: close, 'aria-label': '关闭' }, '×')),
      h('div', { class: 'dialog__body' }, body),
      actions.length ? h('div', { class: 'dialog__foot' }, actions.map(([label, handler, cls]) =>
        h('button', { class: `btn ${cls || ''}`, onclick: () => (handler ? handler(close) : close()) }, label))) : null
    ]));
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });
    document.addEventListener('keydown', onEsc);
    document.body.append(overlay);
    return { close };
  }

  function conflictDialog(conflict) {
    const external = conflict.external || {};
    dialog({
      title: '修订冲突 · 配置已被其他会话修改',
      body: h('div', {}, [
        h('p', { class: 'muted' }, external.note || '服务器上的配置已经变化。'),
        h('div', { class: 'conflict-grid' }, [
          h('div', { class: 'conflict-col' }, [
            h('span', { class: 'eyebrow' }, '本地工作副本'),
            h('span', { class: 'mono' }, S.store.revision),
            h('p', { class: 'muted' }, '包含你的未保存修改'),
            h('span', { class: 'badge badge--warn' }, '未保存')
          ]),
          h('div', { class: 'conflict-col' }, [
            h('span', { class: 'eyebrow' }, '服务器'),
            h('span', { class: 'mono' }, conflict.serverRevision),
            h('ul', { class: 'conflict-changes' }, (external.changes || ['配置已变化']).map((c) => h('li', {}, c)))
          ])
        ]),
        h('p', { class: 'muted' }, '修订号只用于并发控制，不提供配置历史。')
      ]),
      actions: [
        ['以服务器为准（丢弃本地修改）', async (close) => { await S.store.reload(); toast(`已重载服务器配置 · ${S.store.revision}`, 'info'); close(); }, 'btn--danger'],
        ['覆盖保存（保留本地修改）', (close) => { close(); forceSave(); }],
        ['取消', null]
      ]
    });
  }

  /* ---------- 抽屉（右侧编辑面板，非 LuCI 式 modalonly） ---------- */
  function drawer({ eyebrow, title, renderBody, onSubmit, submitLabel = '保存到工作副本', width }) {
    const root = document.querySelector('#drawer-root');
    const close = () => {
      root.classList.remove('is-open');
      document.removeEventListener('keydown', onEsc);
      setTimeout(() => root.replaceChildren(), 240);
    };
    const onEsc = (e) => { if (e.key === 'Escape') close(); };
    const overlay = h('div', { class: 'drawer-overlay', onclick: close });
    const panel = h('div', { class: `drawer ${width ? 'drawer--wide' : ''}` }, [
      h('div', { class: 'drawer__head' }, [
        h('div', {}, h('span', { class: 'drawer__eyebrow' }, eyebrow), h('strong', { class: 'drawer__title' }, title)),
        h('button', { class: 'drawer__close', onclick: close, 'aria-label': '关闭' }, '×')
      ]),
      h('div', { class: 'drawer__body' }),
      h('div', { class: 'drawer__foot' }, [
        h('button', { class: 'btn', onclick: close }, '取消'),
        h('button', { class: 'btn btn--primary', onclick: submit }, submitLabel)
      ])
    ]);
    root.append(overlay, panel);
    const spec = renderBody(panel.querySelector('.drawer__body'));
    function submit() {
      const payload = spec.submit();
      if (payload === false) return;
      Promise.resolve(onSubmit(payload)).then((ok) => { if (ok !== false) close(); });
    }
    document.addEventListener('keydown', onEsc);
    requestAnimationFrame(() => root.classList.add('is-open'));
    return { close };
  }

  /* ---------- 表单件 ---------- */
  function field(label, control, hint) {
    const id = `f-${Math.random().toString(36).slice(2, 7)}`;
    const el = h('div', { class: 'field' });
    if (label) {
      el.append(h('label', { for: id }, label));
      if (control.inputControl) control.inputControl.setAttribute('id', id);
      else if (control.setAttribute) control.setAttribute('id', id);
    }
    el.append(control);
    if (hint) el.append(h('div', { class: 'field__hint' }, hint));
    return el;
  }

  function input({ value = '', placeholder = '', type = 'text', disabled = false, oninput }) {
    const control = h('input', { class: 'input', type, value, placeholder, disabled, oninput });
    if (type !== 'password') return control;

    const button = h('button', {
      class: 'input-reveal__button', type: 'button', 'aria-pressed': 'false',
      onclick: () => {
        const visible = control.type === 'text';
        control.type = visible ? 'password' : 'text';
        button.setAttribute('aria-pressed', String(!visible));
        button.textContent = visible ? '显示' : '隐藏';
      }
    }, '显示');
    const wrapper = h('div', { class: 'input-reveal' }, control, button);
    Object.defineProperty(wrapper, 'value', {
      configurable: false,
      enumerable: true,
      get: () => control.value,
      set: (next) => { control.value = next; }
    });
    wrapper.inputControl = control;
    return wrapper;
  }

  function select(options, value, onchange) {
    return h('select', { class: 'select', onchange: (e) => onchange?.(e.target.value) },
      options.map(([v, label]) => h('option', { value: v, selected: String(v) === String(value) }, label)));
  }

  function toggle(checked, onchange, label) {
    const btn = h('button', {
      class: 'switch', role: 'switch', 'aria-checked': String(!!checked), 'aria-label': label || '',
      onclick: () => {
        const next = btn.getAttribute('aria-checked') !== 'true';
        btn.setAttribute('aria-checked', String(next));
        onchange?.(next);
      }
    });
    return btn;
  }

  function toggleRow(label, checked, onchange) {
    const row = h('div', { class: 'toggle-row' });
    row.append(toggle(checked, onchange, label), h('span', {}, label));
    return row;
  }

  /* chips：动态列表（逗号 / 回车添加，× 删除） */
  function chips(values = [], { placeholder = '', onchange } = {}) {
    let current = asList(values).map(String);
    const box = h('div', { class: 'chips' });
    const list = h('span', { class: 'contents' });
    const inp = h('input', {
      class: 'chips__input', placeholder,
      onkeydown: (e) => {
        if (e.key === 'Enter' || e.key === ',') {
          e.preventDefault();
          const v = inp.value.trim();
          if (v && !current.includes(v)) { current.push(v); render(); onchange?.(current.slice()); }
          inp.value = '';
        }
      }
    });
    function render() {
      list.replaceChildren();
      current.forEach((v, i) => list.append(h('span', { class: 'chip' }, v, h('button', {
        class: 'chip__x', type: 'button', 'aria-label': `移除 ${v}`,
        onclick: () => { current.splice(i, 1); render(); onchange?.(current.slice()); }
      }, '×'))));
    }
    render();
    box.append(list, inp);
    return box;
  }

  /* ---------- Match 编辑器（每行一条表达式 · 行内 OR · geo 前缀补全） ---------- */
  function matchEditor({ value = [], kind, catalog = [], placeholder = '' }) {
    const ta = h('textarea', { class: 'match-editor__ta', spellcheck: 'false', autocomplete: 'off', placeholder });
    ta.value = asList(value).join('\n');
    const list = h('div', { class: 'match-editor__list', hidden: true, role: 'listbox' });
    const status = h('div', { class: 'match-editor__status' });
    const wrap = h('div', { class: 'match-editor' }, ta, list, status);

    const prefixes = kind === 'domain' ? ['full:', 'domain:', 'regexp:', 'geosite:'] : ['geoip:'];
    const geoPrefix = kind === 'domain' ? 'geosite:' : 'geoip:';
    let matches = [];
    let active = 0;

    function currentLine() {
      const pos = ta.selectionStart ?? ta.value.length;
      const start = ta.value.lastIndexOf('\n', pos - 1) + 1;
      let end = ta.value.indexOf('\n', pos);
      if (end < 0) end = ta.value.length;
      return { start, end, value: ta.value.slice(start, end).trim() };
    }

    function render() {
      list.replaceChildren();
      if (!matches.length) { list.hidden = true; status.textContent = ''; return; }
      matches.forEach((m, i) => list.append(h('button', {
        type: 'button', class: 'match-editor__item' + (i === active ? ' is-active' : ''),
        role: 'option', 'aria-selected': String(i === active),
        onmousedown: (e) => e.preventDefault(),
        onclick: () => accept(m)
      }, m)));
      status.textContent = `${matches.length} 个匹配`;
      list.hidden = false;
    }

    function suggest() {
      const line = currentLine().value.toLowerCase();
      matches = [];
      if (line.startsWith(geoPrefix)) {
        const needle = line.slice(geoPrefix.length);
        matches = catalog.filter((n) => String(n).toLowerCase().includes(needle)).slice(0, 50).map((n) => geoPrefix + n);
      } else if (line === '' || prefixes.some((p) => p.startsWith(line))) {
        matches = prefixes.filter((p) => p.startsWith(line));
      }
      active = 0;
      render();
    }

    function accept(value) {
      const line = currentLine();
      ta.value = ta.value.slice(0, line.start) + value + ta.value.slice(line.end);
      const pos = line.start + value.length;
      ta.setSelectionRange(pos, pos);
      ta.focus();
      suggest();
    }

    ta.addEventListener('input', suggest);
    ta.addEventListener('click', suggest);
    ta.addEventListener('focus', suggest);
    ta.addEventListener('blur', () => setTimeout(() => { list.hidden = true; status.textContent = ''; }, 130));
    ta.addEventListener('keydown', (e) => {
      if (list.hidden || !matches.length) return;
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        active = (active + (e.key === 'ArrowDown' ? 1 : -1) + matches.length) % matches.length;
        render();
      } else if (e.key === 'Tab') {
        e.preventDefault();
        accept(matches[active]);
      } else if (e.key === 'Escape') {
        e.preventDefault();
        list.hidden = true;
        status.textContent = '';
      }
    });

    return {
      el: wrap,
      get value() { return ta.value.split('\n').map((s) => s.trim()).filter(Boolean); }
    };
  }

  /* ---------- 校验问题列表 ---------- */
  function issueList(issues, jump, warning = false) {
    if (!issues.length) return null;
    return h('ul', { class: 'issue-list' },
      issues.map((issue) => h('li', { class: `issue ${warning ? 'issue--warning' : ''}` }, [
        h('span', { class: 'issue__code' }, issue.code),
        h('span', { class: 'issue__where' }, [issue.object_type, issue.object_id, issue.option].filter(Boolean).join(' / ') || '全局'),
        h('span', { class: 'issue__msg' }, issue.message),
        jump && issue.object_type ? h('button', { class: 'btn btn--sm', onclick: () => jump(issue) }, '前往') : null
      ])));
  }

  /* ---------- 视图页头 ---------- */
  function viewHead(title, sub, actions = []) {
    return h('div', { class: 'view-head' }, [
      h('div', {}, h('h1', { class: 'view-title' }, title), sub ? h('p', { class: 'view-sub' }, sub) : null),
      actions.length ? h('div', { class: 'view-head__actions' }, actions) : null
    ]);
  }

  /* 下拉补全：把悬空引用保留为可修复选项（与 LuCI 版一致的修复语义） */
  function selectWithMissing(options, current, label) {
    const values = options.slice();
    if (current && !values.some(([v]) => v === current)) values.push([current, `${label || '缺失'}：${current}`]);
    return values;
  }

  Object.assign(S, { ui: { beginRender, beginRoute, isCurrentRoute, renderShell, renderStatusStrip, toast, dialog, conflictDialog, drawer, field, input, select, toggle, toggleRow, chips, matchEditor, issueList, viewHead, selectWithMissing, onValidate, jumpToObject } });
})();
