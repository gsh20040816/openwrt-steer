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

function loadHelper(runtime) {
	const source = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/steer.js'), 'utf8');
	const baseclass = { extend: (value) => value };
	const rpc = {
		declare: ({ method }) => (...args) => {
			if (method == 'commit')
				throw new Error('LuCI must validate and commit through one backend transaction');
			if (method == 'status') {
				runtime.statusCalls++;
				return Promise.resolve(Object.assign({}, runtime.status, {
					last_apply: runtime.commitCalls > 0 ?
						{ sequence: '11', result: runtime.applyResult } :
						{ sequence: '10', result: { ok: true } }
				}));
			}
			if (method == 'validate') {
				runtime.validationCalls++;
				return Promise.resolve(runtime.validation);
			}
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
		getElementById: (id) => id == 'steer-runtime-status' ? runtime.currentStatusNode : (id == 'maincontent' ? runtime.mainContent : null)
	};
	const translate = (value) => String(value);
	const uci = {
		changes: () => Promise.resolve({}),
		revert: (config) => { runtime.revertedConfigs.push(config); return Promise.resolve(); },
		load: (config) => { runtime.loadedConfigs.push(config); return Promise.resolve(); },
		get: (config, section, option) => config == 'steer' && section == 'main' && option == 'enabled' ? runtime.enabled : null
	};
	const window = { setTimeout: (callback) => callback(), location: { pathname: '/cgi-bin/luci/admin/services/steer/nodes', reload: () => { runtime.reloaded = true; } } };

	return new Function('baseclass', 'rpc', 'uci', 'ui', 'E', '_', 'L', 'document', 'window', source)(
		baseclass, rpc, uci, ui, element, translate, L, document, window);
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
		testCalls: [],
		sequence: [],
		notifications: [],
		revertedConfigs: [],
		loadedConfigs: [],
		currentStatusNode: {
			replaceWith: (value) => { runtime.replacement = value; }
		},
		mainContent: { prepend: (value) => { runtime.lifecycleBar = value; } },
		lifecycleState: {
			ok: true, pending: true, pending_apply: false,
			saved: { digest: 'saved-a' }, active: { generation: 'generation-a' }
		}
	};
	const helper = loadHelper(runtime);
	helper.loadStyle();
	await new Promise((resolve) => setImmediate(resolve));
	assert.ok(textContent(runtime.lifecycleBar).includes('Draft / Saved / Active'));
	assert.ok(textContent(runtime.lifecycleBar).includes('Save & Apply pending changes'));
	runtime.lifecycleState = {
		ok: true, pending: false, pending_apply: true,
		saved: { digest: 'saved-b' }, active: { generation: 'generation-old' }
	};
	helper.loadStyle();
	await new Promise((resolve) => setImmediate(resolve));
	assert.ok(textContent(runtime.lifecycleBar).includes('Apply Saved configuration'),
		'every configuration page exposes Apply Saved when the runtime projection is pending');
	await helper.intentPreview(false);
	await helper.intentPreview(true);
	assert.deepEqual(runtime.previewCalls, [ false, true ],
		'Canonical Preview forwards only the explicit temporary reveal decision');
	const addedSections = [];
	const namedSection = {
		sectiontype: 'rule',
		map: {
			config: 'steer',
			data: { add: (...args) => addedSections.push(args) },
			save: () => Promise.resolve('saved')
		}
	};
	helper.configureNamedSection(namedSection);
	assert.equal(namedSection.anonymous, false, 'Steer-owned sections require explicit IDs');
	assert.equal(namedSection.handleAdd(null, 'Rule-A'), undefined,
		'Invalid UCI section IDs fail before saving');
	assert.equal(runtime.notifications.at(-1).level, 'danger');
	await namedSection.handleAdd(null, 'laptop_direct');
	assert.deepEqual(addedSections, [ [ 'steer', 'rule', 'laptop_direct' ] ],
		'Valid section IDs are persisted as named UCI sections');

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
			save: () => {
				savedRuleOrders.push([ ...ruleOrder ]);
				return Promise.resolve('saved');
			}
		}
	};
	helper.configureNamedSection(orderedRuleSection, null, 'default');
	await orderedRuleSection.handleAdd(null, 'first_rule');
	await orderedRuleSection.handleAdd(null, 'second_rule');
	assert.deepEqual(savedRuleOrders, [
		[ 'first_rule', 'default' ],
		[ 'first_rule', 'second_rule', 'default' ]
	], 'Default-only and existing-rule UCI drafts save each new rule immediately before Default');
	assert.deepEqual(moveCalls, [
		[ 'steer', 'first_rule', 'default', false ],
		[ 'steer', 'second_rule', 'default', false ]
	], 'Named rule creation explicitly moves each new UCI section before Default');

	let rendered = helper.renderStatus({ healthy: false }, runtime.validation, false);
	assert.ok(textContent(rendered).includes('Steer is disabled'),
		'An intentionally disabled Steer reports a disabled state');

	rendered = helper.renderStatus({ healthy: false }, runtime.validation, true);
	assert.ok(textContent(rendered).includes('Traffic steering is not healthy'),
		'Live component readiness determines unhealthy state');

	rendered = helper.renderStatus({ healthy: false }, {
		ok: false,
		errors: [ { code: 'DANGLING_ROUTE', object_type: 'rule', object_id: 'broken', option: 'route', message: 'route is missing' } ],
		warnings: []
	}, true);
	const failedText = textContent(rendered);
	assert.ok(failedText.includes('The saved configuration is invalid') && failedText.includes('route is missing'),
		'Invalid saved intent exposes the exact validation issue');

	runtime.status = { healthy: true };
	await helper.apply({ handleSave: () => { runtime.sequence.push('save'); return Promise.resolve(); } }, null, '1');
	assert.deepEqual(runtime.sequence, [ 'save', 'validate', 'commit' ],
		'LuCI backend validates and commits the pending UCI session in one RPC before leaving Apply to procd');
	assert.equal(runtime.indicatorRefreshed, true, 'LuCI refreshes the pending change indicator after commit');
	assert.equal(runtime.modalTitle, 'Applying Steer',
		'Apply shows an explicit transaction progress modal');
	assert.equal(runtime.modalHidden, true, 'Apply closes the progress modal after RPC completion');
	assert.equal(runtime.statusCalls, 2, 'LuCI snapshots the Apply sequence and observes exactly one newer result');
	assert.equal(runtime.validationCalls, 1, 'LuCI validates the candidate once before commit');
	assert.ok(textContent(runtime.replacement).includes('Traffic steering is active'),
		'Apply replaces stale overview status with the final runtime result');
	assert.equal(runtime.notifications.at(-1).level, 'info');

	runtime.sequence = [];
	await helper.applyPending();
	assert.deepEqual(runtime.sequence, [ 'validate', 'commit' ],
		'Overview Save & Apply commits the existing rpcd candidate without inventing a second form save');
	const applySavedResult = await helper.applySaved();
	assert.equal(applySavedResult.generation, 'generation-saved');
	assert.equal(runtime.applySavedCalls, 1, 'Apply Saved uses its restricted RPC without touching pending form data');
	await helper.discardPending();
	assert.deepEqual(runtime.revertedConfigs, [ 'steer' ]);
	assert.deepEqual(runtime.loadedConfigs, [ 'steer' ]);

	runtime.sequence = [];
	runtime.candidateValidation = {
		ok: false,
		errors: [ {
			code: 'GEO_CATEGORY_NOT_FOUND', object_type: 'rule', object_id: 'unknown',
			option: 'domain_match', message: 'geosite category is unavailable'
		} ],
		warnings: []
	};
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
	assert.ok(textContent(runtime.notifications.at(-1).content).includes('geosite category is unavailable'),
		'The backend object-level Geo issue is shown to the user');

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
	assert.ok(textContent(runtime.notifications.at(-1).content).includes('commit status 4'));
	runtime.commitStatus = 0;

	rendered = helper.renderStatus({ healthy: true, ignored_detail: true }, runtime.validation, true);
	assert.ok(!textContent(rendered).includes('ignored_detail'),
		'Overview status renders only the public health contract');
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
