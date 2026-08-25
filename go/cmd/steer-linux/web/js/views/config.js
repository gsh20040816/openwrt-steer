/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 配置 · 高级：Canonical JSON 原文编辑器 + 校验问题映射 + geo 补全。 */
'use strict';
(function () {
  const S = window.S;
  const { h } = S;
  const ui = S.ui;

  function currentGeoToken(value, cursor) {
    const before = String(value || '').slice(0, cursor == null ? String(value || '').length : cursor);
    const match = before.match(/(geosite|geoip):[a-z0-9_!.@-]*$/);
    return match ? { start: before.length - match[0].length, end: before.length, kind: match[1], value: match[0] } : null;
  }

  const view = {
    name: 'config',
    async render(root) {
      const isCurrent = ui.beginRender(root);
      const [geosite, geoip] = await Promise.all([S.api.geodata('geosite'), S.api.geodata('geoip')]);
      const catalogs = { geosite, geoip };

      const editor = h('textarea', { class: 'textarea editor-tall', spellcheck: 'false', autocomplete: 'off' });
      editor.value = JSON.stringify(S.store.intent, null, 2);
      const syntax = h('div', { class: 'match-editor__status' }, 'JSON 语法 OK · 修改后点击下方校验');
      const suggestions = h('div', { class: 'node-groups' });
      const issues = h('div', {});
      const revBadge = h('span', { class: 'badge', title: S.store.revision }, `修订 ${S.fmtRevision(S.store.revision)}`);
      const dirtyBadge = h('span', { class: 'badge badge--warn', hidden: !S.store.dirty }, '工作副本已修改');

      let matches = [];
      const renderSuggestions = () => {
        const token = currentGeoToken(editor.value, editor.selectionStart);
        matches = [];
        suggestions.replaceChildren();
        if (!token) return;
        const needle = token.value.slice(token.kind.length + 1);
        matches = (catalogs[token.kind]?.names || []).filter((n) => String(n).includes(needle)).slice(0, 30).map((n) => `${token.kind}:${n}`);
        matches.forEach((value) => suggestions.append(h('button', {
          class: 'chip', type: 'button',
          onclick: () => {
            editor.value = editor.value.slice(0, token.start) + value + editor.value.slice(token.end);
            const pos = token.start + value.length;
            editor.setSelectionRange(pos, pos);
            renderSuggestions();
            editor.focus();
          }
        }, value)));
      };
      ['input', 'click', 'focus', 'keyup'].forEach((ev) => editor.addEventListener(ev, () => {
        try { JSON.parse(editor.value); syntax.textContent = `JSON 语法 OK · GeoSite ${catalogs.geosite.readable ? catalogs.geosite.count + ' 名称' : 'catalog 不可用'} · GeoIP ${catalogs.geoip.readable ? catalogs.geoip.count + ' 名称' : 'catalog 不可用'}`; }
        catch (e) { syntax.textContent = `JSON 语法错误：${e.message}`; }
        renderSuggestions();
      }));

      async function validate() {
        try {
          const parsed = JSON.parse(editor.value);
          const v = await S.api.validate(parsed);
          const geoErrors = S.validateGeoCategories(parsed, catalogs).map((message) => ({
            code: 'UNKNOWN_GEO_CATEGORY', object_type: 'rule', message
          }));
          v.errors.push(...geoErrors);
          v.ok = v.errors.length === 0;
          issues.replaceChildren(
            h('div', { class: 'card__head validation-head' }, h('div', {}, h('span', { class: 'eyebrow' }, '校验'), h('div', { class: 'card__title' }, `${v.errors.length} 错误 · ${v.warnings.length} 警告`))),
            ui.issueList(v.errors, ui.jumpToObject),
            ui.issueList(v.warnings, ui.jumpToObject, true)
          );
        } catch (e) {
          issues.replaceChildren(h('div', { class: 'alert alert--err' }, e.message));
        }
      }

      async function save(apply) {
        try {
          const parsed = JSON.parse(editor.value);
          const geoErrors = S.validateGeoCategories(parsed, catalogs);
          if (geoErrors.length) throw new Error(geoErrors.join('\n'));
          const normalized = S.store.normalizeIntent(parsed);
          Object.keys(S.store.intent).forEach((k) => delete S.store.intent[k]);
          Object.assign(S.store.intent, normalized);
          S.store.touch();
          const res = await S.store.save(apply);
          if (res.ok) {
            if (apply && !res.res.applied) {
              ui.toast(`已保存，但 Apply 失败：${res.res.apply_result?.error || res.res.error?.message || '运行态未切换'}`, 'err');
            } else {
              ui.toast(apply ? `已保存并 Apply · ${res.res.apply_result?.generation || '已切换'}` : `已保存 · 修订 ${S.store.revision}`, 'ok');
            }
            revBadge.textContent = `修订 ${S.fmtRevision(S.store.revision)}`;
            dirtyBadge.hidden = true;
            editor.value = JSON.stringify(S.store.intent, null, 2);
          } else if (res.conflict) {
            ui.conflictDialog(res.conflict);
          }
        } catch (e) {
          ui.toast(`保存失败：${e.message}`, 'err');
        }
      }

      if (!isCurrent()) return;
      root.append(
		ui.viewHead('配置 · 高级', 'Canonical JSON 原文（schema 9）。结构化视图与本视图共享同一工作副本', [revBadge, dirtyBadge]),
        h('section', { class: 'card' }, [
          h('div', { class: 'editor-actions' }, [
            h('button', { class: 'btn', onclick: validate }, '校验'),
            h('button', { class: 'btn', onclick: () => save(false) }, '保存'),
            h('button', { class: 'btn btn--primary', onclick: () => save(true) }, '保存并 Apply'),
            h('button', {
              class: 'btn', onclick: () => {
                try { editor.value = JSON.stringify(JSON.parse(editor.value), null, 2); ui.toast('已格式化', 'info'); }
                catch (e) { ui.toast(`无法格式化：${e.message}`, 'err'); }
              }
            }, '格式化')
          ]),
          editor,
          syntax,
          suggestions,
          issues
        ])
      );
    }
  };

  S.views = S.views || {};
  S.views.config = view;
})();
