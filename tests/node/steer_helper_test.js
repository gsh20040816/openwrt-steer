#!/usr/bin/env node

'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');

if (typeof String.prototype.format != 'function') {
	Object.defineProperty(String.prototype, 'format', {
		value: function(...values) {
			let index = 0;
			return String(this).replace(/%[sd]/g, () => String(values[index++]));
		}
	});
}

const root = path.resolve(__dirname, '../..');
const validationIssueFixtures = JSON.parse(fs.readFileSync(path.join(root, 'ui/validation-issue-fixtures.json'), 'utf8'));
const formInputFixtures = JSON.parse(fs.readFileSync(path.join(root, 'ui/form-input-fixtures.json'), 'utf8'));
const creationPolicyFixtures = JSON.parse(fs.readFileSync(path.join(root, 'ui/creation-policy-fixtures.json'), 'utf8'));
const uiSpec = JSON.parse(fs.readFileSync(path.join(root, 'ui/steer-ui-spec.json'), 'utf8'));

function element(tag, attributes, children) {
	if (Array.isArray(tag)) {
		children = attributes;
		attributes = {};
	}
	else if (arguments.length == 2 &&
		(Array.isArray(attributes) || typeof attributes != 'object' || attributes == null)) {
		children = attributes;
		attributes = {};
	}

	return {
		tag,
		attributes: attributes || {},
		children: children == null ? [] : (Array.isArray(children) ? children : [ children ]),
		addEventListener(name, listener) { this.listeners = this.listeners || {}; this.listeners[name] = listener; },
		prepend(child) { this.children.unshift(child); },
		querySelector(selector) {
			return findElement(this, (candidate) => selector == 'div' && candidate.tag == 'div');
		},
		remove() { this.removed = true; }
	};
}

function textContent(value) {
	if (value == null)
		return '';
	if (typeof value != 'object')
		return String(value);
	return (value.children || []).map(textContent).join(' ');
}

function findElement(value, predicate) {
	if (value == null || typeof value != 'object') return null;
	if (predicate(value)) return value;
	for (const child of value.children || []) {
		const found = findElement(child, predicate);
		if (found) return found;
	}
	return null;
}

function loadHelper(runtime) {
	const source = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/steer.js'), 'utf8');
	const baseclass = { extend: (value) => value };
	const rpc = {
		declare: ({ object, method }) => (...args) => {
			if (method == 'commit')
				throw new Error('LuCI must validate and commit through one backend transaction');
			if (method == 'discard_candidate') {
				runtime.revertedConfigs.push('steer');
				return Promise.resolve({ ok: true });
			}
			if (method == 'status') {
				runtime.statusCalls++;
				const status = Object.assign({}, runtime.status, {
					last_apply: runtime.commitCalls > 0 ?
						{ sequence: '11', result: runtime.applyResult } :
						{ sequence: '10', result: { ok: true } }
				});
				if (runtime.commitCalls > 0 && runtime.afterApplyGeneration !== undefined)
					status.generation = runtime.afterApplyGeneration;
				return Promise.resolve(status);
			}
			if (method == 'validate') {
				runtime.validationCalls++;
				return Promise.resolve(runtime.validation);
			}
			if (method == 'access')
				return Promise.resolve(runtime.permissions?.[args[2]] !== false);
			if (method == 'commit_candidate') {
				runtime.sequence.push('validate');
				runtime.validationCalls++;
				const code = runtime.candidateValidation.ok === true ? runtime.commitStatus : null;
				const reply = runtime.commitReply;
				const committed = code === 0;
				if (committed) {
					runtime.sequence.push('commit');
					runtime.commitCalls++;
				}
				return Promise.resolve({
					committed,
					validation: runtime.candidateValidation,
					error: code == null || code === 0 ? null : `commit status ${code}; reply ${JSON.stringify(reply)}`
				});
			}
			if (method == 'intent_preview') {
				runtime.previewCalls.push(args[0]);
				return Promise.resolve(runtime.previewResponse);
			}
			if (method == 'overview_state')
				return Promise.resolve(runtime.lifecycleState);
			if (method == 'apply_saved') {
				runtime.applySavedCalls++;
				return Promise.resolve(runtime.applySavedResult);
			}
			if (method == 'overview_probe' || method == 'route_speedtest' || method == 'node_speedtest') {
				runtime.testCalls.push({ method, args });
				return Promise.resolve({ ok: true });
			}
			if (method == 'apply')
				throw new Error('LuCI must not start a second Apply after UCI commit triggered procd');
			return Promise.resolve({});
		}
	};
	const ui = {
		changes: {
			apply: () => { throw new Error('ui.changes.apply() is non-awaitable and must not be used'); },
			renderChangeIndicator: () => { runtime.indicatorRefreshed = true; }
		},
		showModal: (title, content) => { runtime.modalTitle = title; runtime.modalContent = content; },
		hideModal: () => { runtime.modalHidden = true; },
		addNotification: (title, content, level) => runtime.notifications.push({ title, content, level }),
		createHandlerFn: (context, handler) => (...args) => handler.apply(context, args)
	};
	const L = {
		resolveDefault: (promise, fallback) => Promise.resolve(promise).catch(() => fallback),
		resource: (value) => value
	};
	const document = {
		head: { appendChild: () => {} },
		getElementById: (id) => id == 'steer-lifecycle-global' ? runtime.lifecycleBar : (id == 'maincontent' ? runtime.mainContent : null)
	};
	const translate = (value) => String(value);
	const uci = {
		changes: () => Promise.resolve({}),
		save: () => { runtime.sequence.push('save-candidate'); return Promise.resolve(); },
		unload: (config) => { runtime.unloadedConfigs.push(config); },
		load: (config) => { runtime.loadedConfigs.push(config); return Promise.resolve(); },
		get: (config, section, option) => {
			if (config != 'steer') return null;
			if (section == 'main' && option == 'enabled') return runtime.enabled;
			const value = runtime.uciSections?.[section];
			return option == null ? value : value?.[option];
		},
		set: (config, section, option, value) => {
			if (config == 'steer' && section == 'main' && option == 'enabled') {
				runtime.enabled = value;
				runtime.lifecycleState.desired.enabled = value == '1';
				runtime.lifecycleState.pending = true;
				runtime.sequence.push('set-enabled');
			}
		}
	};
	const window = { setTimeout: (callback) => callback(), location: { pathname: '/cgi-bin/luci/admin/services/steer/nodes', reload: () => { runtime.reloaded = true; } } };

	return new Function('baseclass', 'rpc', 'uci', 'ui', 'uiSpec', 'E', '_', 'L', 'document', 'window', source)(
		baseclass, rpc, uci, ui, uiSpec, element, translate, L, document, window);
}

async function main() {
	const runtime = {
		status: {},
		validation: { ok: true, errors: [], warnings: [] },
		candidateValidation: { ok: true, errors: [], warnings: [] },
		commitStatus: 0,
		commitReply: { result: 'standard UCI commit reply' },
		enabled: '1',
		applyResult: { ok: true, output: 'applied' },
		applySavedResult: { ok: true, generation: 'generation-saved' },
		applySavedCalls: 0,
		statusCalls: 0,
		validationCalls: 0,
		commitCalls: 0,
		previewCalls: [],
			previewResponse: { ok: true, source: 'committed', redacted: true, intent: {} },
			permissions: {},
		testCalls: [],
		sequence: [],
		notifications: [],
		revertedConfigs: [],
		unloadedConfigs: [],
		loadedConfigs: [],
		mainContent: { prepend: (value) => { runtime.lifecycleBar = value; } },
		lifecycleState: {
			ok: true, pending: true, pending_apply: false,
			desired: { enabled: true, validation: { ok: true, errors: [], warnings: [] } },
			saved: { enabled: true, digest: 'saved-a' }, active: { generation: 'generation-a', healthy: true }
		}
	};
	const helper = loadHelper(runtime);
	for (const fixture of formInputFixtures.cases) {
		assert.equal(helper.validateInput(fixture.format, fixture.value) === true, fixture.valid,
			`${fixture.format} must classify ${JSON.stringify(fixture.value)} from the shared fixture`);
	}
	for (const fixture of creationPolicyFixtures.cases) {
		const expected = { ...fixture.expected };
		delete expected.id;
		const expectedUCI = Object.fromEntries(Object.entries(expected).map(([key, value]) => [
			key, typeof value == 'boolean' ? (value ? '1' : '0') : String(value)
		]));
		assert.deepEqual(helper.creationDefaults(fixture.collection, fixture.overrides), expectedUCI,
			`${fixture.collection} helper defaults must match the shared Canonical fixture`);
	}
	const ambiguous = helper.disambiguateReferences(creationPolicyFixtures.ambiguous_references.nodes.map((node) => ({
		id: node.id, label: node.name, detail: `${node.server}:${node.server_port}`
	})));
	assert.equal(ambiguous.find((item) => item.id == 'node-unique').label, 'Unique');
	assert.match(ambiguous.find((item) => item.id == 'node-a1b2c3').label, /Same · a\.example:1080 · Duplicate 1/);
	runtime.permissions.node_import = false;
	assert.deepEqual(await helper.permissions([ 'node_import', 'node_speedtest' ]), {
		node_import: false, node_speedtest: true
	}, 'each handwritten action checks its own ubus method permission');
	runtime.permissions.node_import = true;
	runtime.permissions.write = false;
	assert.deepEqual(await helper.permissions([ 'node_import' ], true), { node_import: true, uci_write: false },
		'Node import separately requires UCI write access in addition to parser RPC access');
	runtime.permissions.write = true;
	assert.equal(helper.rpcErrorText({ error_code: 'MISSING_NODE_ID', error: 'raw backend detail' }), 'A Node ID is required.',
		'RPC failures are localized by stable code before raw detail');
	assert.equal(helper.rpcErrorText({ error_code: 'IMPORT_PROGRAM_MISSING' }), 'The Steer node parser is not installed.');
	assert.equal(helper.rpcErrorText({ error_code: 'IMPORT_PROGRAM_NOT_EXECUTABLE' }), 'The Steer node parser is not executable.');
	const currentView = { handleSave: () => { runtime.sequence.push('capture-form'); return Promise.resolve(); } };
	helper.loadStyle(currentView);
	await new Promise((resolve) => setImmediate(resolve));
	assert.ok(textContent(runtime.lifecycleBar).includes('Steer status'));
	assert.ok(textContent(runtime.lifecycleBar).includes('Enable Steer'));
	assert.ok(textContent(runtime.lifecycleBar).includes('Working copy'));
	assert.ok(textContent(runtime.lifecycleBar).includes('Save & Apply pending changes'));
	assert.equal(findElement(runtime.lifecycleBar, (node) => node.attributes?.role == 'switch')?.attributes?.['aria-checked'], 'true',
		'the global status area exposes the desired Enable state as an accessible switch');
	runtime.lifecycleState = {
		ok: true, pending: false, pending_apply: true,
		desired: { enabled: true, validation: { ok: true, errors: [], warnings: [] } },
		saved: { enabled: true, digest: 'saved-b' }, active: { generation: 'generation-old', healthy: true }
	};
	helper.loadStyle(currentView);
	await new Promise((resolve) => setImmediate(resolve));
	assert.ok(textContent(runtime.lifecycleBar).includes('Apply Saved configuration'),
		'every configuration page exposes Apply Saved when the runtime projection is pending');
	runtime.sequence = [];
	runtime.lifecycleState.pending = false;
	runtime.lifecycleState.pending_apply = false;
	await helper.setGlobalEnabled(false, currentView);
	assert.deepEqual(runtime.sequence, [ 'capture-form', 'set-enabled', 'save-candidate', 'validate', 'commit' ],
		'global Enable captures the current form, writes main.enabled, then validates, commits and applies one complete candidate');
	assert.equal(runtime.enabled, '0');
	runtime.lifecycleState = {
		ok: true, pending: true, pending_apply: false,
		desired: { enabled: false, validation: { ok: false, errors: [ { code: 'DANGLING_ROUTE' } ], warnings: [] } },
		saved: { enabled: true }, active: { generation: 'generation-a', healthy: true }
	};
	helper.loadStyle(currentView);
	await new Promise((resolve) => setImmediate(resolve));
	const invalidToggle = findElement(runtime.lifecycleBar, (node) => node.attributes?.role == 'switch');
	assert.equal(invalidToggle.attributes.disabled, true);
	assert.equal(invalidToggle.attributes.title, 'Fix validation errors before changing Steer.',
		'an invalid candidate blocks global Enable with a stable reason');
	runtime.lifecycleState = {
		ok: true, pending: false, pending_apply: true,
		desired: { enabled: false, validation: { ok: true, errors: [], warnings: [] } },
		saved: { enabled: false }, active: { generation: 'generation-a', healthy: true }
	};
	helper.loadStyle(currentView);
	await new Promise((resolve) => setImmediate(resolve));
	const partialText = textContent(runtime.lifecycleBar);
	assert.ok(partialText.includes('Saved configuration') && partialText.includes('Disabled') &&
		partialText.includes('Active configuration') && partialText.includes('Running normally'),
		'the global status area keeps Saved Enable separate from the actual Active runtime');
	runtime.sequence = [];
	runtime.commitCalls = 0;
	runtime.statusCalls = 0;
	runtime.validationCalls = 0;
	runtime.enabled = '1';
	runtime.lifecycleState = {
		ok: true, pending: false, pending_apply: false,
		desired: { enabled: true, validation: { ok: true, errors: [], warnings: [] } },
		saved: { enabled: true }, active: { generation: 'generation-a', healthy: true }
	};
	await helper.intentPreview(false);
	await helper.intentPreview(true);
	assert.deepEqual(runtime.previewCalls, [ false, true ],
		'Canonical Preview forwards only the explicit temporary reveal decision');
	const addedSections = [];
	const removedSections = [];
	const modalSections = [];
	const sectionValues = {};
	const namedData = {
		add: (...args) => { addedSections.push(args); return args[2]; },
		set: (_config, sectionId, option, value) => { (sectionValues[sectionId] ||= {})[option] = value; },
		remove: (_config, sectionId) => { removedSections.push(sectionId); delete sectionValues[sectionId]; }
	};
	const namedSection = {
		sectiontype: 'rule',
		map: {
			config: 'steer',
			data: namedData,
			save: () => { throw new Error('Add must not persist a provisional section before modal Save'); }
		},
		handleAdd: function(_ev, sectionId) {
			const added = this.map.data.add(this.map.config, this.sectiontype, sectionId);
			this.map.addedSection = added;
			return this.renderMoreOptionsModal(added);
		},
		renderMoreOptionsModal: function(sectionId) {
			modalSections.push({ sectionId, values: { ...(sectionValues[sectionId] || {}) } });
			return Promise.resolve(sectionId);
		},
		handleModalCancel: function(_modalMap, _ev, isSaving) {
			if (this.map.addedSection != null && !isSaving)
				this.map.data.remove(this.map.config, this.map.addedSection);
			delete this.map.addedSection;
		}
	};
	helper.configureNamedSection(namedSection, { enabled: '1' });
	assert.equal(namedSection.anonymous, false, 'Steer-owned sections retain stable named IDs');
	assert.equal(namedSection.autoIDs, true, 'regular Add generates the stable ID instead of asking the user');
	assert.equal(namedSection.handleAdd(null, 'Rule-A'), undefined,
		'Invalid UCI section IDs fail before saving');
	assert.equal(runtime.notifications.at(-1).level, 'danger');
	await namedSection.handleAdd(null, 'laptop_direct');
	assert.deepEqual(addedSections, [ [ 'steer', 'rule', 'laptop_direct' ] ],
		'Valid section IDs create one provisional named UCI section');
	assert.deepEqual(modalSections, [ { sectionId: 'laptop_direct', values: {
		enabled: '1', default: '0', dns_profile: '', route: 'direct'
	} } ],
		'Grid Add opens the native editor modal with defaults already available');
	namedSection.handleModalCancel(null, null, false);
	assert.deepEqual(removedSections, [ 'laptop_direct' ],
		'Cancelling the native editor removes the provisional section without a pending row');

	const ruleOrder = [ 'default' ];
	const savedRuleOrders = [];
	const moveCalls = [];
	const orderedRuleSection = {
		sectiontype: 'rule',
		map: {
			config: 'steer',
			data: {
				add: (config, type, sectionId) => {
					assert.deepEqual([ config, type ], [ 'steer', 'rule' ]);
					ruleOrder.push(sectionId);
					return sectionId;
				},
				set: () => {},
				move: (config, sectionId, targetId, after) => {
					moveCalls.push([ config, sectionId, targetId, after ]);
					const from = ruleOrder.indexOf(sectionId);
					if (from < 0) return false;
					ruleOrder.splice(from, 1);
					const target = ruleOrder.indexOf(targetId);
					if (target < 0) return false;
					ruleOrder.splice(target + (after ? 1 : 0), 0, sectionId);
					return true;
				}
			},
			save: () => { throw new Error('Add must wait for the modal Save action'); }
		},
		handleAdd: function(_ev, sectionId) {
			const added = this.map.data.add(this.map.config, this.sectiontype, sectionId);
			this.map.addedSection = added;
			return this.renderMoreOptionsModal(added);
		},
		renderMoreOptionsModal: function(sectionId) {
			savedRuleOrders.push([ ...ruleOrder ]);
			return Promise.resolve(sectionId);
		}
	};
	helper.configureNamedSection(orderedRuleSection, null, 'default');
	await orderedRuleSection.handleAdd(null, 'first_rule');
	await orderedRuleSection.handleAdd(null, 'second_rule');
	assert.deepEqual(savedRuleOrders, [
		[ 'first_rule', 'default' ],
		[ 'first_rule', 'second_rule', 'default' ]
	], 'Each new Rule opens its modal only after being provisionally ordered before Default');
	assert.deepEqual(moveCalls, [
		[ 'steer', 'first_rule', 'default', false ],
		[ 'steer', 'second_rule', 'default', false ]
	], 'Named rule creation explicitly moves each new UCI section before Default');

	runtime.uciSections = {
		node_a: { '.name': 'node_a', source_subscription: '' },
		feed_a: { '.name': 'feed_a', source_subscription: 'feed' },
		node_b: { '.name': 'node_b', source_subscription: '' }
	};
	const nodeOrder = [ 'node_a', 'feed_a', 'node_b' ];
	const orderingMoves = [];
	const orderedNodeSection = {
		sectiontype: 'node',
		map: {
			config: 'steer',
			data: {
				move: (config, sectionId, targetId, after) => {
					orderingMoves.push([ config, sectionId, targetId, after ]);
					const sourceIndex = nodeOrder.indexOf(sectionId);
				nodeOrder.splice(sourceIndex, 1);
				const targetIndex = nodeOrder.indexOf(targetId);
				nodeOrder.splice(targetIndex + (after ? 1 : 0), 0, sectionId);
			}
			}
		},
		cfgsections: () => nodeOrder.filter((sectionId) => runtime.uciSections[sectionId].source_subscription == ''),
		renderRowActions: () => element('td', {}, element('div'))
	};
	helper.configureOrdering(orderedNodeSection, 'nodes');
	const nodeBActions = orderedNodeSection.renderRowActions('node_b');
	const moveUp = findElement(nodeBActions, (candidate) => candidate.attributes?.['data-steer-order'] == 'up');
	assert.equal(moveUp.attributes.disabled, false, 'a movable non-boundary row exposes an enabled Move up action');
	moveUp.attributes.click({ preventDefault() {}, stopPropagation() {} });
	assert.deepEqual(orderingMoves, [ [ 'steer', 'node_b', 'node_a', false ] ],
		'the real LuCI ordering helper writes exactly one pending UCI move by stable section ID');
	assert.deepEqual(nodeOrder, [ 'node_b', 'node_a', 'feed_a' ],
		'Node moves stay within the visible source group without changing source ownership');
	const firstActions = orderedNodeSection.renderRowActions('node_b');
	assert.equal(findElement(firstActions, (candidate) => candidate.attributes?.['data-steer-order'] == 'up').attributes.disabled, true,
		'the real LuCI ordering helper disables a move at the visible group boundary');
	delete runtime.uciSections;

	runtime.status = { healthy: true };
	runtime.lifecycleState.active = { generation: 'generation-new', healthy: true };
	await helper.apply({ handleSave: () => { runtime.sequence.push('save'); return Promise.resolve(); } }, null, '1');
	await new Promise((resolve) => setImmediate(resolve));
	assert.deepEqual(runtime.sequence, [ 'save', 'validate', 'commit' ],
		'LuCI backend validates and commits the pending UCI session in one RPC before leaving Apply to procd');
	assert.equal(runtime.indicatorRefreshed, true, 'LuCI refreshes the pending change indicator after commit');
	assert.equal(runtime.modalTitle, 'Applying Steer',
		'Apply shows an explicit transaction progress modal');
	assert.equal(runtime.modalHidden, true, 'Apply closes the progress modal after RPC completion');
	assert.equal(runtime.statusCalls, 2, 'LuCI snapshots the Apply sequence and observes exactly one newer result');
	assert.equal(runtime.validationCalls, 1, 'LuCI validates the candidate once before commit');
	assert.ok(textContent(runtime.lifecycleBar).includes('Running normally'),
		'Apply refreshes the global lifecycle status from the final runtime result');
	assert.equal(runtime.notifications.at(-1).level, 'info');

	runtime.sequence = [];
	runtime.commitCalls = 0;
	runtime.applyResult = { ok: false, activated: false, error: 'start candidate: systemd refused start' };
	const activationFailure = await helper.applyPending();
	assert.equal(activationFailure.saved, true);
	assert.equal(activationFailure.active_unchanged, true);
	const activationNotice = runtime.notifications.at(-1);
	assert.equal(activationNotice.title, 'Configuration saved; application failed');
	assert.ok(textContent(activationNotice.content).includes('previous running configuration') &&
		textContent(activationNotice.content).includes('Apply Saved configuration') &&
		!textContent(activationNotice.content).includes('systemd refused start'),
		'activation failure explains commit-before-apply and the recovery action on every page');

	runtime.sequence = [];
	runtime.commitCalls = 0;
	runtime.applyResult = { ok: true, activated: false };
	runtime.enabled = '0';
	const disabledResult = await helper.applyPending();
	assert.equal(disabledResult.saved, true);
	assert.ok(textContent(runtime.notifications.at(-1).content).includes('runtime resources were cleaned up'),
		'disabled cleanup has a distinct successful result');
	runtime.enabled = '1';
	runtime.applyResult = { ok: true, activated: true };

	runtime.sequence = [];
	runtime.commitCalls = 0;
	runtime.status.generation = 'generation-old';
	runtime.afterApplyGeneration = 'generation-candidate';
	runtime.applyResult = { ok: false, activated: true, error: 'candidate became unhealthy' };
	const changedActiveFailure = await helper.applyPending();
	assert.equal(changedActiveFailure.active_unchanged, false);
	assert.equal(runtime.notifications.at(-1).title, 'Configuration saved; application failed');
	assert.ok(textContent(runtime.notifications.at(-1).content).includes('Check Diagnostics before relying on traffic stability') &&
		!textContent(runtime.notifications.at(-1).content).includes('generation-candidate'),
		'a partial activation never falsely promises that the previous Active generation is still carrying traffic');
	delete runtime.afterApplyGeneration;
	delete runtime.status.generation;
	runtime.applyResult = { ok: true, activated: true };

	runtime.sequence = [];
	await helper.applyPending();
	assert.deepEqual(runtime.sequence, [ 'validate', 'commit' ],
		'Overview Save & Apply commits the existing rpcd candidate without inventing a second form save');
	const applySavedResult = await helper.applySaved();
	assert.equal(applySavedResult.generation, 'generation-saved');
	assert.equal(runtime.applySavedCalls, 1, 'Apply Saved uses its restricted RPC without touching pending form data');
	await helper.discardPending();
	assert.deepEqual(runtime.revertedConfigs, [ 'steer' ]);
	assert.deepEqual(runtime.unloadedConfigs, [ 'steer' ]);
	assert.deepEqual(runtime.loadedConfigs, [ 'steer' ]);

	runtime.sequence = [];
	runtime.commitCalls = 0;
	runtime.candidateValidation = { ok: true, errors: [], warnings: validationIssueFixtures.validation.warnings };
	await helper.applyPending();
	assert.equal(runtime.notifications.at(-1).level, 'warning');
	assert.ok(textContent(runtime.notifications.at(-1).content).includes('connection-stage') &&
		!textContent(runtime.notifications.at(-1).content).includes('DNS_PROJECTION_EMPTY'),
		'non-blocking write warnings remain visible after a successful Apply');

	runtime.sequence = [];
	runtime.candidateValidation = validationIssueFixtures.validation;
	const commitsBeforeInvalidCandidate = runtime.commitCalls;
	const invalidResult = await helper.apply({
		handleSave: () => { runtime.sequence.push('save'); return Promise.resolve(); }
	}, null, '1');
	assert.deepEqual(runtime.sequence, [ 'save', 'validate' ],
		'An invalid candidate is backend-validated without reaching UCI commit');
	assert.equal(runtime.commitCalls, commitsBeforeInvalidCandidate,
		'Unknown Geo selectors remain pending and are never persisted');
	assert.equal(invalidResult.saved, false);
	assert.equal(runtime.notifications.at(-1).level, 'danger');
	const validationText = textContent(runtime.notifications.at(-1).content);
	for (const issue of [ ...runtime.candidateValidation.errors, ...runtime.candidateValidation.warnings ]) {
		assert.ok(!validationText.includes(issue.code) && !validationText.includes(issue.object_id));
		const optionLabel = { node: 'Node', route: 'Route', dns_profile: 'DNS profile' }[issue.option];
		if (optionLabel) assert.ok(validationText.includes(optionLabel));
		else assert.ok(!validationText.includes(issue.option));
	}
	assert.ok(validationText.includes('Go to field'),
		'Every located write issue offers direct object/field navigation');
	for (const secret of validationIssueFixtures.forbidden_message_values) assert.ok(!validationText.includes(secret));

	runtime.sequence = [];
	runtime.candidateValidation = { ok: true, errors: [], warnings: [] };
	runtime.commitStatus = 4;
	const commitsBeforeRPCFailure = runtime.commitCalls;
	const commitFailure = await helper.apply({
		handleSave: () => { runtime.sequence.push('save'); return Promise.resolve(); }
	}, null, '1');
	assert.deepEqual(runtime.sequence, [ 'save', 'validate' ],
		'A nonzero ubus callback status is not confused with its reply object');
	assert.equal(runtime.commitCalls, commitsBeforeRPCFailure,
		'A failed standard UCI commit is never reported as committed');
	assert.equal(commitFailure.saved, false);
	assert.ok(textContent(runtime.notifications.at(-1).content).includes('Operation failed.') &&
		!textContent(runtime.notifications.at(-1).content).includes('commit status 4'));
	runtime.commitStatus = 0;

	await helper.overviewProbe('direct');
	await helper.routeSpeedtest('route_a', true);
	await helper.speedtest('node_a', false);
	assert.deepEqual(runtime.testCalls, [
		{ method: 'overview_probe', args: [ 'direct' ] },
		{ method: 'route_speedtest', args: [ 'route_a', true ] },
		{ method: 'node_speedtest', args: [ 'node_a', false ] }
	], 'LuCI helper forwards every diagnostic argument unchanged');

	console.log('Steer LuCI helper regression tests passed.');
}

main().catch((error) => {
	console.error(error.stack || error);
	process.exit(1);
});
