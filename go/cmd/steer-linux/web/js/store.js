/* SPDX-License-Identifier: GPL-3.0-or-later */
/* 工作副本 store：Intent 文档 + 修订号 + dirty 状态 + Save/Apply 状态机。 */
'use strict';
(function () {
  const S = window.S;

  let intent = null;
  let revision = '';
  let dirty = false;
  let platform = null;
  let platformRevision = '';
  let overview = null;

  const listeners = new Set();
  const emit = () => { for (const fn of listeners) fn(); };
  const LIST_FIELDS = ['nodes', 'subscriptions', 'routes', 'dns_profiles', 'local_proxies', 'rules'];
  const normalizeIntent = (value) => {
    if (!value || typeof value !== 'object') return value;
    LIST_FIELDS.forEach((field) => { if (!Array.isArray(value[field])) value[field] = []; });
    return value;
  };

  const store = {
    get intent() { return intent; },
    get revision() { return revision; },
    get dirty() { return dirty; },
    get platform() { return platform; },
    get platformRevision() { return platformRevision; },
    get overview() { return overview; },

    normalizeIntent,

    subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); },

    async init() {
      const [config, plat, ov] = await Promise.all([S.api.config(), S.api.platform(), S.api.overview()]);
      intent = normalizeIntent(config.intent);
      revision = config.revision;
      platform = plat.settings;
      platformRevision = plat.revision;
      overview = ov;
      emit();
    },

    touch() { dirty = true; emit(); },
    async refreshOverview() { overview = await S.api.overview(); emit(); },

    /* 保存（可选 Apply）。修订冲突以 { ok:false, conflict } 返回，由 UI 弹冲突对话框。 */
    async save(apply, force = false) {
      try {
        const res = await S.api.putConfig(intent, force ? null : revision, apply);
        revision = res.revision;
        dirty = false;
        await store.refreshOverview();
        emit();
        return { ok: true, res };
      } catch (err) {
        if (err.code === 'CONFLICT') { emit(); return { ok: false, conflict: err }; }
        throw err;
      }
    },

    /* 以服务器为准：丢弃本地修改。 */
    async reload() {
      const config = await S.api.config();
      intent = normalizeIntent(config.intent);
      revision = config.revision;
      dirty = false;
      emit();
    },

    async savePlatform() {
      const res = await S.api.putPlatform(platform, platformRevision);
      platformRevision = res.revision;
      emit();
      return res;
    }
  };

  S.store = store;
})();
