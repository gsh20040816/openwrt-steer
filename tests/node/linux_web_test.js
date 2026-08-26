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
    this.value = '';
    this.classList = {
      add: (...names) => this.setClasses([...classSet(this), ...names]),
      remove: (...names) => this.setClasses([...classSet(this)].filter((name) => !names.includes(name))),
      toggle: (name, force) => {
        const names = classSet(this);
        const enabled = force == null ? !names.has(name) : !!force;
        if (enabled) names.add(name); else names.delete(name);
        this.setClasses([...names]);
        return enabled;
      },
      contains: (name) => classSet(this).has(name)
    };
  }

  setClasses(names) { this.className = [...new Set(names.filter(Boolean))].join(' '); }
  append(...children) {
    this.children.push(...children.flat(Infinity).filter((child) => child != null));
    this.syncSelectValue();
  }
  replaceChildren(...children) {
    this.children = children.flat(Infinity).filter((child) => child != null);
    this.syncSelectValue();
  }
  setAttribute(name, value) {
    const normalized = String(value);
    this.attributes[name] = normalized;
    if (['value', 'type', 'placeholder', 'rows', 'id'].includes(name)) this[name] = normalized;
    if (name === 'selected') this.selected = true;
    if (name === 'disabled') this.disabled = true;
    if (name === 'hidden') this.hidden = true;
  }
  getAttribute(name) { return this.attributes[name]; }
  addEventListener(name, listener) { this.listeners[name] = listener; }
  removeEventListener() {}
  remove() { this.removed = true; }
  focus() {}
  setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; }

  syncSelectValue() {
    if (this.tag !== 'select') return;
    const options = this.children.filter((child) => child instanceof Element && child.tag === 'option');
    const selected = options.find((option) => option.selected) || options[0];
    if (selected) this.value = selected.value;
  }

  querySelector(selector) {
    return find(this, (element) => element !== this && matches(element, selector));
  }

  querySelectorAll(selector) { return findAll(this, (element) => element !== this && matches(element, selector)); }
}

function classSet(element) {
  return new Set(String(element.className || '').split(/\s+/).filter(Boolean));
}

function matches(element, selector) {
  const simple = selector.trim().split(/\s+/).pop();
  if (simple.startsWith('.')) return simple.slice(1).split('.').every((name) => classSet(element).has(name));
  if (simple.startsWith('#')) return element.attributes.id === simple.slice(1);
  return element.tag === simple;
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

function createEnvironment(save, intent = { main: { enabled: true } }) {
  const strip = new Element('div');
  const toasts = new Element('div');
  const view = new Element('main');
  const drawerRoot = new Element('div');
  const body = new Element('body');
  strip.setAttribute('id', 'strip');
  toasts.setAttribute('id', 'toasts');
  view.setAttribute('id', 'view');
  drawerRoot.setAttribute('id', 'drawer-root');
  body.append(strip, toasts, view, drawerRoot);
  const document = {
    body,
    querySelector: (selector) => body.querySelector(selector),
    querySelectorAll: (selector) => body.querySelectorAll(selector),
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
    fmtReport: () => ({ ok: true, label: 'ok', detail: '' }),
    uid: (prefix) => `${prefix}-test`,
    api: {
      async geodata() { return { names: [], readable: false, count: 0 }; },
      async speedtestNode() { return { results: [] }; }
    },
    store: {
      intent,
      overview: {},
      revision: 'revision-1',
      dirty: false,
      save,
      touch() { touchCount++; this.dirty = true; }
    }
  };
  const window = { S };
  const source = fs.readFileSync(path.join(root, 'go/cmd/steer-linux/web/js/ui.js'), 'utf8');
  new Function('window', 'document', 'setTimeout', 'requestAnimationFrame', source)(window, document, () => 0, (callback) => callback());
  return { S, window, document, body, view, drawerRoot, get touchCount() { return touchCount; } };
}

function loadView(environment, name) {
  const source = fs.readFileSync(path.join(root, `go/cmd/steer-linux/web/js/views/${name}.js`), 'utf8');
  new Function('window', 'document', 'setTimeout', source)(environment.window, environment.document, () => 0);
}

function fieldWithLabel(rootElement, label) {
  return findAll(rootElement, (element) => classSet(element).has('field')).find((element) => {
    const labelElement = element.children.find((child) => child instanceof Element && child.tag === 'label');
    return labelElement && text(labelElement) === label;
  });
}

function buttonWithText(rootElement, label) {
  return find(rootElement, (element) => element.tag === 'button' && text(element) === label);
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
    chips: () => Object.assign(new Element('div'), { commitPending: () => false }),
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

function testChipsCommitPendingTokensConsistently() {
  const environment = createEnvironment(async () => ({ ok: true }));
  const updates = [];
  const control = environment.S.ui.chips(['existing'], { onchange: (values) => updates.push(values) });
  const input = find(control, (element) => classSet(element).has('chips__input'));
  const add = find(control, (element) => classSet(element).has('chips__add'));

  input.value = 'blurred';
  input.listeners.blur();
  assert.deepStrictEqual(updates.at(-1), ['existing', 'blurred'], 'blur must commit the pending token');

  const updateCount = updates.length;
  input.value = '  ';
  input.listeners.blur();
  input.value = 'existing';
  add.listeners.click();
  assert.strictEqual(updates.length, updateCount, 'blank and duplicate tokens must not be added');

  let commaPrevented = false;
  input.value = 'literal,comma';
  input.listeners.keydown({ key: ',', preventDefault() { commaPrevented = true; } });
  assert.strictEqual(commaPrevented, false, 'comma must not act as a global chip delimiter');
  input.listeners.blur();
  assert.deepStrictEqual(updates.at(-1), ['existing', 'blurred', 'literal,comma'], 'commas in field content must survive');

  input.value = 'added';
  add.listeners.click();
  assert.deepStrictEqual(updates.at(-1), ['existing', 'blurred', 'literal,comma', 'added'], 'explicit Add must share the commit path');

  let enterPrevented = false;
  input.value = 'entered';
  input.listeners.keydown({ key: 'Enter', preventDefault() { enterPrevented = true; } });
  assert.ok(enterPrevented, 'Enter must submit the pending token without submitting the surrounding form');
  assert.deepStrictEqual(updates.at(-1), ['existing', 'blurred', 'literal,comma', 'added', 'entered'], 'Enter must share the commit path');
}

function representativeIntent(privateKey) {
  return {
    main: { enabled: true },
    subscriptions: [],
    nodes: [{
      id: 'ssh-node', enabled: true, name: 'SSH node', type: 'ssh', server: 'ssh.example', server_port: 22,
      username: 'root', password: 'password-secret', private_key: privateKey,
      host_key_algorithms: ['ssh-ed25519']
    }],
    routes: [{ id: 'direct', enabled: true, kind: 'direct' }],
    dns_profiles: [{ id: 'public', enabled: true, name: 'Public' }],
    local_proxies: [],
    rules: [
      { id: 'rule-one', enabled: true, name: 'Rule one', dns_profile: 'public', route: 'direct' },
      { id: 'default', enabled: true, default: true, name: 'Default', dns_profile: 'public', route: 'direct' }
    ]
  };
}

function openOnlyEditor(environment, viewName) {
  environment.S.views[viewName].render(environment.view);
  const edit = buttonWithText(environment.view, '编辑');
  assert.ok(edit, `${viewName} view must expose its editor`);
  return edit.listeners.click();
}

function testNodeStringListSubmitAndPrivateKeyRoundTrip() {
  const originalKey = '-----BEGIN OPENSSH PRIVATE KEY-----\noriginal\n-----END OPENSSH PRIVATE KEY-----\n';
  const editedKey = '-----BEGIN OPENSSH PRIVATE KEY-----\nline one\n\nline two  \n-----END OPENSSH PRIVATE KEY-----\n';
  const environment = createEnvironment(async () => ({ ok: true }), representativeIntent(originalKey));
  loadView(environment, 'nodes');
  openOnlyEditor(environment, 'nodes');

  let privateKeyField = fieldWithLabel(environment.drawerRoot, '私钥');
  let privateKeyInput = find(privateKeyField, (element) => element.tag === 'textarea');
  assert.ok(privateKeyInput, 'multiline sensitive private_key must render as a textarea');
  assert.ok(privateKeyInput.classList.contains('is-masked'), 'private_key must be masked by default');
  assert.strictEqual(privateKeyInput.value, originalKey, 'the editor must preserve the loaded key including its trailing newline');

  const reveal = find(privateKeyField, (element) => classSet(element).has('input-reveal__button'));
  reveal.listeners.click();
  assert.ok(!privateKeyInput.classList.contains('is-masked'), 'private_key must reveal only after the explicit control is used');
  assert.strictEqual(reveal.getAttribute('aria-pressed'), 'true', 'reveal state must be exposed accessibly');

  privateKeyInput.value = editedKey;
  privateKeyInput.listeners.input({ target: privateKeyInput });

  const algorithmsField = fieldWithLabel(environment.drawerRoot, 'Host key algorithms');
  const pendingAlgorithm = find(algorithmsField, (element) => classSet(element).has('chips__input'));
  pendingAlgorithm.value = 'ssh-rsa';

  const protocolField = fieldWithLabel(environment.drawerRoot, '协议');
  const protocol = find(protocolField, (element) => element.tag === 'select');
  protocol.value = 'trojan';
  protocol.listeners.change({ target: protocol });
  protocol.value = 'ssh';
  protocol.listeners.change({ target: protocol });

  privateKeyField = fieldWithLabel(environment.drawerRoot, '私钥');
  privateKeyInput = find(privateKeyField, (element) => element.tag === 'textarea');
  assert.strictEqual(privateKeyInput.value, editedKey, 'switching SSH authentication fields must not clear private_key');
  const passwordField = fieldWithLabel(environment.drawerRoot, '密码');
  const password = find(passwordField, (element) => element.tag === 'input');
  assert.strictEqual(password.value, 'password-secret', 'switching away from and back to SSH must not clear password');

  const currentAlgorithmsField = fieldWithLabel(environment.drawerRoot, 'Host key algorithms');
  const finalPending = find(currentAlgorithmsField, (element) => classSet(element).has('chips__input'));
  finalPending.value = 'rsa-sha2-512';
  buttonWithText(environment.drawerRoot, '保存到工作副本').listeners.click();

  const savedNode = environment.S.store.intent.nodes[0];
  assert.strictEqual(savedNode.private_key, editedKey, 'drawer submit must preserve private_key bytes and newlines');
  assert.strictEqual(savedNode.password, 'password-secret', 'saving private-key authentication must not clear the password secret');
  assert.deepStrictEqual(
    savedNode.host_key_algorithms,
    ['ssh-ed25519', 'ssh-rsa', 'rsa-sha2-512'],
    'node string-list pending tokens must flush on rebuild and drawer submit'
  );

  const reloaded = createEnvironment(async () => ({ ok: true }), JSON.parse(JSON.stringify(environment.S.store.intent)));
  loadView(reloaded, 'nodes');
  openOnlyEditor(reloaded, 'nodes');
  const reloadedKey = find(fieldWithLabel(reloaded.drawerRoot, '私钥'), (element) => element.tag === 'textarea');
  assert.strictEqual(reloadedKey.value, editedKey, 'save and reload must retain every private-key newline');
  assert.ok(reloadedKey.classList.contains('is-masked'), 'a reloaded private key must return to the masked state');
}

async function testRuleStringListFlushesOnDrawerSubmit() {
  const environment = createEnvironment(async () => ({ ok: true }), representativeIntent('key'));
  loadView(environment, 'rules');
  await openOnlyEditor(environment, 'rules');
  const inbound = fieldWithLabel(environment.drawerRoot, 'inbound（本地代理端点）');
  const pending = find(inbound, (element) => classSet(element).has('chips__input'));
  pending.value = 'proxy-last';
  buttonWithText(environment.drawerRoot, '保存到工作副本').listeners.click();
  assert.deepStrictEqual(
    environment.S.store.intent.rules[0].inbound,
    ['proxy-last'],
    'rule drawer submit must atomically flush its last pending chip'
  );
}

Promise.resolve()
  .then(testFailedToggleRestoresDraft)
  .then(testConflictRestoresUntilOverwriteIsChosen)
  .then(testNewRulesAreStoredBeforeDefault)
  .then(testChipsCommitPendingTokensConsistently)
  .then(testNodeStringListSubmitAndPrivateKeyRoundTrip)
  .then(testRuleStringListFlushesOnDrawerSubmit)
  .then(() => console.log('Linux web regression tests passed.'))
  .catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
