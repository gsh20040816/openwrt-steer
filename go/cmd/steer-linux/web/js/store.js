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
  let mutationEpoch = 0;
  let stateEpoch = 0;
  let saving = false;
  let reloading = false;
  let applying = false;
  let overview = null;
  let runtime = null;
  let externalRevision = '';
  let lastRefreshedAt = '';

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
    mutationEpoch++;
  };
  const snapshotIntent = () => JSON.parse(JSON.stringify(intent));
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
    get draftEpoch() { return mutationEpoch; },
    get saving() { return saving; },
    get reloading() { return reloading; },
    get applying() { return applying; },
    get listenerCount() { return listeners.size; },
    get pendingApply() { return overview?.pending_apply === true; },
    get overview() { return overview; },
    get runtime() { return runtime; },
    get externalRevision() { return externalRevision; },
    get hasExternalChange() { return externalRevision !== '' && externalRevision !== revision; },
    get lastRefreshedAt() { return lastRefreshedAt; },

    normalizeIntent,

    subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); },

    async init() {
      const [config, ov, runtimeInfo] = await Promise.all([S.api.config(), S.api.overview(), S.api.runtime()]);
      installIntent(config.intent);
      revision = config.revision;
      overview = ov;
      runtime = runtimeInfo;
      externalRevision = ov.saved_revision && ov.saved_revision !== revision ? ov.saved_revision : '';
      lastRefreshedAt = new Date().toISOString();
      emit();
    },

    touch() {
      draftText = serializeIntent();
      draftError = '';
      mutationEpoch++;
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
      mutationEpoch++;
      dirty = true;
      emit();
      return { ok: draftError === '', error: draftError };
    },
    async refreshOverview() {
      const expectedState = stateEpoch;
      const refreshed = await S.api.overview();
      if (expectedState !== stateEpoch) return { ok: false, superseded: true };
      overview = refreshed;
      externalRevision = refreshed.saved_revision && refreshed.saved_revision !== revision ? refreshed.saved_revision : '';
      lastRefreshedAt = new Date().toISOString();
      emit();
      return { ok: true };
    },

    async refreshServerState() {
      if (saving || reloading || applying) return { ok: false, busy: true };
      const expectedState = stateEpoch;
      const [refreshedOverview, refreshedRuntime] = await Promise.all([S.api.overview(), S.api.runtime()]);
      if (expectedState !== stateEpoch) return { ok: false, superseded: true };
      overview = refreshedOverview;
      runtime = refreshedRuntime;
      externalRevision = refreshedOverview.saved_revision && refreshedOverview.saved_revision !== revision ? refreshedOverview.saved_revision : '';
      lastRefreshedAt = new Date().toISOString();
      emit();
      return { ok: true, changed: externalRevision !== '', revision: refreshedOverview.saved_revision || '' };
    },

    /* 保存（可选 Apply）。修订冲突以 { ok:false, conflict } 返回，由 UI 弹冲突对话框。 */
    async save(apply, force = false) {
      if (draftError) {
        const error = new Error(`当前 JSON Draft 无效：${draftError}`);
        error.code = 'INVALID_DRAFT';
        throw error;
      }
      if (saving || reloading || applying) return { ok: false, busy: true };
      const savedSnapshot = snapshotIntent();
      const savedRevision = force ? null : revision;
      const startedMutation = mutationEpoch;
      const expectedState = ++stateEpoch;
      saving = true;
      emit();
      try {
        let res;
        try {
          res = await S.api.putConfig(savedSnapshot, savedRevision, apply);
        } catch (err) {
          if (expectedState !== stateEpoch) return { ok: false, superseded: true };
          const staleDraft = mutationEpoch !== startedMutation;
          if (err.code === 'CONFLICT') { emit(); return { ok: false, conflict: err, staleDraft }; }
          err.staleDraft = staleDraft;
          throw err;
        }

        /* PUT 已成功时先提交本地修订状态；overview 只是后续的状态刷新。 */
        revision = res.revision;
        externalRevision = '';
        const staleDraft = mutationEpoch !== startedMutation;
        if (!staleDraft) {
          dirty = false;
          draftText = serializeIntent();
        }
        emit();

        let overviewError = null;
        try {
          const refreshedOverview = await S.api.overview();
          if (expectedState === stateEpoch) {
            overview = refreshedOverview;
            emit();
          }
        } catch (error) {
          overviewError = error;
        }
        return { ok: true, res, staleDraft, overviewError };
      } finally {
        saving = false;
        emit();
      }
    },

    async applySaved() {
      if (saving || reloading || applying) return { ok: false, busy: true };
      ++stateEpoch;
      applying = true;
      emit();
      try {
        const result = await S.api.applySaved();
        let overviewError = null;
        try {
          await store.refreshOverview();
        } catch (error) {
          overviewError = error;
        }
        return { ...result, overviewError };
      } finally {
        applying = false;
        emit();
      }
    },

    /* 以服务器为准：丢弃本地修改。 */
    async reload() {
      if (saving || reloading || applying) return { ok: false, busy: true };
      const operation = ++stateEpoch;
      const startedMutation = mutationEpoch;
      reloading = true;
      emit();
      try {
        const [config, ov] = await Promise.all([S.api.config(), S.api.overview()]);
        if (operation !== stateEpoch) return { ok: false, superseded: true };
        if (mutationEpoch !== startedMutation) return { ok: false, staleDraft: true };
        installIntent(config.intent);
        revision = config.revision;
        externalRevision = '';
        overview = ov;
        lastRefreshedAt = new Date().toISOString();
        dirty = false;
        emit();
        return { ok: true };
      } finally {
        if (operation === stateEpoch) {
          reloading = false;
          emit();
        }
      }
    }
  };

  S.store = store;
})();
