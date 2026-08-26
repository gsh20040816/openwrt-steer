/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 工作副本 store：Intent 文档 + 修订号 + dirty 状态 + Save/Apply 状态机。 */
'use strict';
(function () {
  const S = window.S;

  let intent = null;
  let revision = '';
  let dirty = false;
  let draftText = '';
  let draftError = '';
  let overview = null;
  let runtime = null;

  const listeners = new Set();
  const emit = () => { for (const fn of listeners) fn(); };
  const LIST_FIELDS = ['nodes', 'subscriptions', 'routes', 'dns_profiles', 'local_proxies', 'rules'];
  const normalizeIntent = (value) => {
    if (!value || typeof value !== 'object') return value;
    LIST_FIELDS.forEach((field) => { if (!Array.isArray(value[field])) value[field] = []; });
    return value;
  };
  const serializeIntent = () => JSON.stringify(intent, null, 2);
  const installIntent = (value) => {
    intent = normalizeIntent(value);
    draftText = serializeIntent();
    draftError = '';
  };
  const parseDraft = (text) => {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      throw new Error('Canonical JSON 顶层必须是对象');
    }
    if (!parsed.main || typeof parsed.main !== 'object' || Array.isArray(parsed.main) ||
        !parsed.bootstrap || typeof parsed.bootstrap !== 'object' || Array.isArray(parsed.bootstrap)) {
      throw new Error('Canonical JSON 必须包含 main 与 bootstrap 对象');
    }
    return normalizeIntent(parsed);
  };

  const store = {
    get intent() { return intent; },
    get revision() { return revision; },
    get dirty() { return dirty; },
    get draftText() { return draftText; },
    get draftValid() { return draftError === ''; },
    get draftError() { return draftError; },
    get pendingApply() { return overview?.pending_apply === true; },
    get overview() { return overview; },
    get runtime() { return runtime; },

    normalizeIntent,

    subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); },

    async init() {
      const [config, ov, runtimeInfo] = await Promise.all([S.api.config(), S.api.overview(), S.api.runtime()]);
      installIntent(config.intent);
      revision = config.revision;
      overview = ov;
      runtime = runtimeInfo;
      emit();
    },

    touch() {
      draftText = serializeIntent();
      draftError = '';
      dirty = true;
      emit();
    },
    editJSON(text) {
      draftText = String(text ?? '');
      try {
        intent = parseDraft(draftText);
        draftError = '';
      } catch (error) {
        draftError = error.message;
      }
      dirty = true;
      emit();
      return { ok: draftError === '', error: draftError };
    },
    async refreshOverview() { overview = await S.api.overview(); emit(); },

    /* 保存（可选 Apply）。修订冲突以 { ok:false, conflict } 返回，由 UI 弹冲突对话框。 */
    async save(apply, force = false) {
      if (draftError) {
        const error = new Error(`当前 JSON Draft 无效：${draftError}`);
        error.code = 'INVALID_DRAFT';
        throw error;
      }
      try {
        const res = await S.api.putConfig(intent, force ? null : revision, apply);
        revision = res.revision;
        dirty = false;
        draftText = serializeIntent();
        await store.refreshOverview();
        emit();
        return { ok: true, res };
      } catch (err) {
        if (err.code === 'CONFLICT') { emit(); return { ok: false, conflict: err }; }
        throw err;
      }
    },

    async applySaved() {
      const result = await S.api.applySaved();
      await store.refreshOverview();
      return result;
    },

    /* 以服务器为准：丢弃本地修改。 */
    async reload() {
      const [config, ov] = await Promise.all([S.api.config(), S.api.overview()]);
      installIntent(config.intent);
      revision = config.revision;
      overview = ov;
      dirty = false;
      emit();
    }
  };

  S.store = store;
})();
