#!/usr/bin/env node

'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '../..');
const uiSpec = JSON.parse(fs.readFileSync(path.join(root, 'ui/steer-ui-spec.json'), 'utf8'));

class Element {
  constructor(tag) {
    this.tag = tag;
    this.children = [];
    this.attributes = {};
    this.dataset = {};
    this.listeners = {};
    this.className = '';
    this.classList = { add() {}, remove() {}, toggle() {} };
  }

  append(...children) { this.children.push(...children.flat(Infinity).filter((child) => child != null)); }
  replaceChildren(...children) { this.children = children.flat(Infinity).filter((child) => child != null); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name]; }
  addEventListener(name, listener) { this.listeners[name] = listener; }
  removeEventListener() {}
  remove() { this.removed = true; }

  querySelector(selector) {
    if (selector === '.strip__toggle .switch') return find(this, (element) => element.className === 'switch');
    return null;
  }
}

function find(value, predicate) {
  if (value instanceof Element && predicate(value)) return value;
  if (!(value instanceof Element)) return null;
  for (const child of value.children) {
    const match = find(child, predicate);
    if (match) return match;
  }
  return null;
}

function findAll(value, predicate) {
  if (!(value instanceof Element)) return [];
  const matches = predicate(value) ? [value] : [];
  return matches.concat(value.children.flatMap((child) => findAll(child, predicate)));
}

function text(value) {
  if (!(value instanceof Element)) return String(value ?? '');
  return value.children.map(text).join('');
}

function createEnvironment(save) {
  const strip = new Element('div');
  const toasts = new Element('div');
  const body = new Element('body');
  const document = {
    body,
    querySelector(selector) {
      if (selector === '#strip') return strip;
      if (selector === '#toasts') return toasts;
      throw new Error(`unexpected selector ${selector}`);
    },
    querySelectorAll() { return []; },
    addEventListener() {},
    removeEventListener() {}
  };
  const h = (tag, attributes = {}, ...children) => {
    const element = new Element(tag);
    for (const [name, value] of Object.entries(attributes)) {
      if (value == null || value === false) continue;
      if (name === 'class') element.className = value;
      else if (name === 'dataset') Object.assign(element.dataset, value);
      else if (name.startsWith('on')) element.addEventListener(name.slice(2), value);
      else element.setAttribute(name, value === true ? '' : value);
    }
    element.append(...children);
    return element;
  };
  let touchCount = 0;
  const S = {
    uiSpec,
    h,
    icon: () => new Element('span'),
    asList: (value) => value || [],
    fmtRevision: (value) => value,
    store: {
      intent: { main: { enabled: true } },
      overview: {},
      revision: 'revision-1',
      dirty: false,
      save,
      touch() { touchCount++; this.dirty = true; }
    }
  };
  const window = { S };
  const source = fs.readFileSync(path.join(root, 'go/cmd/steer-linux/web/js/ui.js'), 'utf8');
  new Function('window', 'document', 'setTimeout', source)(window, document, () => 0);
  return { S, body, get touchCount() { return touchCount; } };
}

function createRulesEnvironment(intent) {
  const rootElement = new Element('main');
  const document = {
    querySelector(selector) {
      if (selector === '#view') return rootElement;
      throw new Error(`unexpected selector ${selector}`);
    },
    querySelectorAll() { return []; }
  };
  const h = (tag, attributes = {}, ...children) => {
    const element = new Element(tag);
    for (const [name, value] of Object.entries(attributes)) {
      if (value == null || value === false) continue;
      if (name === 'class') element.className = value;
      else if (name === 'dataset') Object.assign(element.dataset, value);
      else if (name.startsWith('on')) element.addEventListener(name.slice(2), value);
      else element.setAttribute(name, value === true ? '' : value);
    }
    element.append(...children);
    return element;
  };
  let nextID = 1;
  let touchCount = 0;
  const ui = {
    beginRender: (root) => root.replaceChildren(),
    viewHead: (title, subtitle, actions) => h('header', {}, title, subtitle, actions),
    input: ({ value }) => Object.assign(new Element('input'), { value }),
    toggle: () => new Element('button'),
    selectWithMissing: (options) => options,
    select: (options, value) => Object.assign(new Element('select'), { value }),
    chips: () => new Element('div'),
    matchEditor: ({ value }) => ({ el: new Element('textarea'), value: value || [] }),
    field: (label, control, help) => h('label', {}, label, control, help),
    toast: () => {},
    drawer(options) {
      const editor = options.renderBody(new Element('div'));
      const rule = editor.submit();
      if (rule !== false) options.onSubmit(rule);
    }
  };
  const S = {
    uiSpec,
    h,
    asList: (value) => value == null ? [] : (Array.isArray(value) ? value : [value]),
    uid: () => `rule_${nextID++}`,
    api: { geodata: () => Promise.resolve({ readable: true, names: [], count: 0 }) },
    ui,
    store: {
      intent,
      touch() { touchCount++; }
    }
  };
  const window = { S };
  const source = fs.readFileSync(path.join(root, 'go/cmd/steer-linux/web/js/views/rules.js'), 'utf8');
  new Function('window', 'document', source)(window, document);

  return {
    S,
    root: rootElement,
    render: () => S.views.rules.render(rootElement),
    async addRule() {
      const button = find(rootElement, (element) => element.tag === 'button' && text(element) === '添加规则');
      assert.ok(button, 'Rules view must expose its add action');
      button.listeners.click();
      await new Promise((resolve) => setImmediate(resolve));
    },
    renderedRuleIDs: () => findAll(rootElement, (element) => element.className.split(' ').includes('rule-row'))
      .map((element) => element.dataset.ruleId),
    get touchCount() { return touchCount; }
  };
}

async function testFailedToggleRestoresDraft() {
  const environment = createEnvironment(async () => { throw new Error('network failed'); });
  await environment.S.ui.onToggleEnabled(false);
  assert.strictEqual(environment.S.store.intent.main.enabled, true, 'failed toggle must restore the prior draft value');
  assert.strictEqual(environment.S.store.dirty, false, 'failed toggle must preserve the prior clean state');
  assert.strictEqual(environment.touchCount, 0, 'immediate toggle must not manufacture a dirty draft before saving');
}

async function testConflictRestoresUntilOverwriteIsChosen() {
  let calls = 0;
  const environment = createEnvironment(async () => {
    calls++;
    if (calls === 1) return { ok: false, conflict: { serverRevision: 'revision-2', external: {} } };
    return { ok: true, res: { applied: true } };
  });
  await environment.S.ui.onToggleEnabled(false);
  assert.strictEqual(environment.S.store.intent.main.enabled, true, 'conflicted toggle must restore the prior draft value');

  const overwrite = find(environment.body, (element) => element.tag === 'button' && text(element).includes('覆盖保存'));
  assert.ok(overwrite, 'conflict dialog must offer an explicit overwrite action');
  overwrite.listeners.click();
  await Promise.resolve();
  await Promise.resolve();

  assert.strictEqual(environment.S.store.intent.main.enabled, false, 'overwrite must reapply the requested toggle');
  assert.strictEqual(environment.touchCount, 1, 'overwrite must mark the reapplied toggle dirty before saving');
  assert.strictEqual(calls, 2, 'overwrite must issue one forced save after the conflicted save');
}

async function testNewRulesAreStoredBeforeDefault() {
  const defaultRule = { id: 'default', enabled: true, default: true, dns_profile: 'direct_dns', route: 'direct' };
  let environment = createRulesEnvironment({
    rules: [defaultRule],
    dns_profiles: [{ id: 'direct_dns', name: 'Direct DNS' }],
    routes: [{ id: 'direct', name: 'Direct' }]
  });
  environment.render();
  await environment.addRule();
  await environment.addRule();
  assert.deepEqual(environment.S.store.intent.rules.map((rule) => rule.id), ['rule_1', 'rule_2', 'default'],
    'A default-only draft stores consecutive new rules in first-match order before Default');
  assert.deepEqual(environment.renderedRuleIDs(), ['rule_1', 'rule_2'],
    'The rendered ordinary-rule order matches the JSON draft order');
  assert.strictEqual(environment.touchCount, 2, 'Each inserted rule marks the draft dirty once');

  environment = createRulesEnvironment({
    rules: [
      { id: 'existing_a', enabled: true, name: 'Existing A', dns_profile: 'direct_dns', route: 'direct' },
      { id: 'existing_b', enabled: true, name: 'Existing B', dns_profile: 'direct_dns', route: 'direct' },
      { ...defaultRule }
    ],
    dns_profiles: [{ id: 'direct_dns', name: 'Direct DNS' }],
    routes: [{ id: 'direct', name: 'Direct' }]
  });
  environment.render();
  await environment.addRule();
  assert.deepEqual(environment.S.store.intent.rules.map((rule) => rule.id),
    ['existing_a', 'existing_b', 'rule_1', 'default'],
    'A new rule follows existing ordinary rules while remaining before Default');
  assert.deepEqual(environment.renderedRuleIDs(), ['existing_a', 'existing_b', 'rule_1'],
    'Existing and newly inserted rules retain the same storage and display order');
}

Promise.resolve()
  .then(testFailedToggleRestoresDraft)
  .then(testConflictRestoresUntilOverwriteIsChosen)
  .then(testNewRulesAreStoredBeforeDefault)
  .then(() => console.log('Linux web regression tests passed.'))
  .catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
