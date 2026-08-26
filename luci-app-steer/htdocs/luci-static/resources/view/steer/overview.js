/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require form';
'require uci';
'require ui';
'require view';
'require steer as steer';
'require steer.ui-spec as uiSpec';

function testResult(report, kind) {
	const result = report?.results?.[0];
	if (!report?.ok || !result?.ok) {
		return E('div', { 'class': 'steer-test-card__result is-error' }, [
			E('strong', {}, _('Failed')),
			E('small', {}, _('See diagnostic logs for details.'))
		]);
	}
	if (kind == 'speedtest') {
		const mbps = result.downloaded_bytes > 0 && result.download_milliseconds > 0 ?
			result.downloaded_bytes * 8 / result.download_milliseconds / 1000 : 0;
		return E('div', { 'class': 'steer-test-card__result is-success' }, [
			E('strong', {}, _('%s Mbps').format(mbps.toFixed(1))),
			E('small', {}, _('%s bytes in %s ms · HTTP %s').format(result.downloaded_bytes, result.download_milliseconds, result.status))
		]);
	}
	return E('div', { 'class': 'steer-test-card__result is-success' }, [
		E('strong', {}, _('%s ms').format(result.first_byte_milliseconds)),
		E('small', {}, _('Connect %s ms · TLS %s ms · HTTP %s · %s attempt(s)').format(
			result.connect_milliseconds || 0, result.tls_milliseconds || 0, result.status, result.attempts))
	]);
}

function renderTestCard(kind, title, description) {
	const output = E('div', { 'class': 'steer-test-card__output' }, E('span', {}, _('Not tested')));
	const button = E('button', {
		'class': 'btn cbi-button-action',
		'click': function(ev) {
			ev.preventDefault();
			const current = ev.currentTarget;
			current.disabled = true;
			output.replaceChildren(E('span', { 'class': 'spinning' }, _('Testing…')));
			return steer.overviewProbe(kind).then((report) => {
				output.replaceChildren(testResult(report, kind));
				current.disabled = false;
				return report;
			}).catch((error) => {
				output.replaceChildren(testResult({ ok: false, error: String(error) }, kind));
				current.disabled = false;
			});
		}
	}, _('Run test'));
	return E('section', { 'class': 'steer-test-card' }, [
		E('div', {}, [ E('h3', {}, title), E('p', {}, description) ]),
		button,
		output
	]);
}

function renderOverviewTests() {
	return E('div', { 'class': 'steer-test-grid' }, [
		renderTestCard('direct', _('Direct target'), _('Visits the configured direct test target under the current Active rules.')),
		renderTestCard('proxy', _('Proxy target'), _('Visits the configured proxy test target under the current Active rules.')),
		renderTestCard('speedtest', _('Download target'), _('Visits the configured download test target under the current Active rules.'))
	]);
}

function diagnosticFact(label, value) {
	return E('div', {}, [ E('dt', {}, label), E('dd', {}, value == null || value === '' ? '—' : String(value)) ]);
}

function hasPendingSteerChanges(changes) {
	if (Array.isArray(changes)) return changes.length > 0;
	if (Array.isArray(changes?.steer)) return changes.steer.length > 0;
	return changes?.steer != null && Object.keys(changes.steer).length > 0;
}

function reportIsStale(report, diagnostics, pending) {
	if (report.scope == 'overview')
		return !report.active_generation || !report.active_digest ||
			report.active_generation != diagnostics.active_generation || report.active_digest != diagnostics.active_digest;
	return pending || !report.saved_digest || report.saved_digest != diagnostics.saved_digest;
}

function renderProbeReport(report, diagnostics, pending) {
	const result = report?.results?.[0] || {};
	const stale = reportIsStale(report, diagnostics, pending);
	const rate = result.downloaded_bytes > 0 && result.download_milliseconds > 0
		? (result.downloaded_bytes * 8 / result.download_milliseconds / 1000).toFixed(1) + ' Mbps' : '—';
	const target = report.scope == 'overview' ? 'Overview' : '%s/%s'.format(report.scope, report.object_id || '—');
	return E('article', { 'class': 'cbi-section' }, [
		E('h4', {}, [ target, ' · ', report.kind, ' · ', stale ? _('Stale') : (report.ok ? _('Succeeded') : _('Failed')) ]),
		E('dl', { 'class': 'steer-status__facts' }, [
			diagnosticFact('tested_at', report.tested_at), diagnosticFact('URL', result.url),
			diagnosticFact(_('Attempts'), result.attempts), diagnosticFact('Connect', result.connect_milliseconds == null ? null : result.connect_milliseconds + ' ms'),
			diagnosticFact('TLS', result.tls_milliseconds == null ? null : result.tls_milliseconds + ' ms'),
			diagnosticFact('TTFB', result.first_byte_milliseconds == null ? null : result.first_byte_milliseconds + ' ms'),
			diagnosticFact('HTTP', result.status), diagnosticFact(_('Bytes'), result.downloaded_bytes),
			diagnosticFact(_('Rate'), rate)
		]),
		(report.error || result.error) ? E('p', { 'class': 'alert-message danger' }, report.error || result.error) : ''
	]);
}

function renderDiagnostics(status, validation, diagnostics, changes) {
	const pending = hasPendingSteerChanges(changes);
	const lastApply = status?.last_apply;
	const result = lastApply?.result;
	return E([], [
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Connectivity targets')),
			E('p', {}, _('Targets are visited under the current Active rules. Success only proves that the URL was reachable at that time; it does not prove a particular outbound, DNS resolver, or absence of DNS leaks.')),
			renderOverviewTests()
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Recent Overview, Node and Route probe reports')),
			...(diagnostics?.warnings || []).map((warning) => E('p', { 'class': 'alert-message warning' }, warning)),
			...((diagnostics?.reports || []).length ? diagnostics.reports.map((report) => renderProbeReport(report, diagnostics, pending)) : [ E('p', {}, _('No saved probe reports.')) ])
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Validation')),
			...((validation?.errors || []).map((issue) => E('p', { 'class': 'alert-message danger' }, '%s · %s/%s · %s'.format(issue.code, issue.object_type || 'configuration', issue.object_id || '—', issue.message)))),
			...((validation?.warnings || []).map((issue) => E('p', { 'class': 'alert-message warning' }, '%s · %s/%s · %s'.format(issue.code, issue.object_type || 'configuration', issue.object_id || '—', issue.message)))),
			validation?.ok ? E('p', {}, _('The committed configuration is valid.')) : ''
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Recent Apply')),
			E('dl', { 'class': 'steer-status__facts' }, [
				diagnosticFact(_('Sequence'), lastApply?.sequence), diagnosticFact(_('Result'), result?.ok ? _('Succeeded') : (lastApply ? _('Failed') : '—')),
				diagnosticFact('Generation', result?.generation), diagnosticFact(_('Error'), result?.error)
			])
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Recent logs')),
			E('pre', { 'class': 'steer-canonical-preview' }, diagnostics?.logs || diagnostics?.log_error || _('No Steer log entries were returned.'))
		]),
		E('button', { 'class': 'btn cbi-button-action', click: function() { window.location.reload(); } }, _('Refresh diagnostics'))
	]);
}

function lifecycleCounts(counts) {
	counts = counts || {};
	return _('Nodes %d · Routes %d · DNS %d · Proxies %d · Rules %d').format(
		counts.nodes || 0, counts.routes || 0, counts.dns_profiles || 0, counts.local_proxies || 0, counts.rules || 0);
}

function renderLifecycleOverview(state) {
	const desired = state?.desired || {};
	const saved = state?.saved || {};
	const active = state?.active || {};
	const pending = state?.pending === true;
	const lastApply = active.last_apply;
	const lastResult = lastApply?.result;
	const actionState = E('p', { 'class': 'steer-lifecycle-action' });
	const runAction = function(button, action, success) {
		button.disabled = true;
		action().then((result) => {
			if (result?.ok === false) throw new Error(result.error || _('Operation failed.'));
			actionState.replaceChildren(success);
			window.location.reload();
		}).catch((error) => {
			button.disabled = false;
			actionState.replaceChildren(String(error));
		});
	};
	const actions = [];
	if (pending) {
		const apply = E('button', { 'class': 'btn cbi-button-positive' }, _('Save & Apply pending changes'));
		const discard = E('button', { 'class': 'btn cbi-button-negative' }, _('Discard pending changes'));
		apply.addEventListener('click', () => runAction(apply, () => steer.applyPending(), _('Pending changes were saved and applied.')));
		discard.addEventListener('click', () => runAction(discard, () => steer.discardPending(), _('Pending changes were discarded.')));
		actions.push(apply, ' ', discard, actionState);
	}
	else if (state?.pending_apply === true) {
		const applySaved = E('button', { 'class': 'btn cbi-button-positive' }, _('Apply Saved configuration'));
		applySaved.addEventListener('click', () => runAction(applySaved, () => steer.applySaved(), _('Saved configuration was applied.')));
		actions.push(applySaved, actionState);
	}
	const desiredWarnings = desired.validation?.warnings || [];
	const savedWarnings = saved.validation?.warnings || [];
	return E([], [
		E('section', { 'class': 'cbi-section steer-lifecycle' }, [
			E('h3', {}, _('Draft / Saved / Active')),
			pending
				? E('p', { 'class': 'alert-message warning' }, _('Pending desired values differ from Saved. Active traffic is unchanged until Save & Apply succeeds.'))
				: E('p', {}, _('There are no pending Steer changes. Desired and Saved are aligned.')),
			E('div', { 'class': 'steer-test-grid' }, [
				E('article', { 'class': 'steer-test-card' }, [
					E('h4', {}, _('Pending desired')),
					E('dl', { 'class': 'steer-status__facts' }, [
						diagnosticFact(_('Pending'), pending ? _('Yes') : _('No')),
						diagnosticFact(_('Enabled'), desired.enabled ? _('Enabled') : _('Disabled')),
						diagnosticFact(_('Objects'), lifecycleCounts(desired.counts)),
						diagnosticFact(_('Validation'), desired.validation?.ok ? _('Valid') : _('Invalid'))
					])
				]),
				E('article', { 'class': 'steer-test-card' }, [
					E('h4', {}, _('Saved configuration')),
					E('dl', { 'class': 'steer-status__facts' }, [
						diagnosticFact(_('Saved desired'), saved.enabled ? _('Enabled') : _('Disabled')),
						diagnosticFact(_('Saved revision'), saved.digest ? saved.digest.slice(0, 12) : '—'),
						diagnosticFact('pending_apply', state?.pending_apply ? _('Yes') : _('No')),
						diagnosticFact(_('Objects'), lifecycleCounts(saved.counts)),
						diagnosticFact(_('Validation'), saved.validation?.ok ? _('Valid') : _('Invalid'))
					])
				]),
				E('article', { 'class': 'steer-test-card' }, [
					E('h4', {}, _('Active runtime')),
					E('dl', { 'class': 'steer-status__facts' }, [
						diagnosticFact(_('Running'), active.generation ? _('Yes') : _('No')),
						diagnosticFact(_('Healthy'), active.healthy ? _('Yes') : _('No')),
						diagnosticFact('Generation', active.generation),
						diagnosticFact('Intent digest', active.intent_digest ? active.intent_digest.slice(0, 12) : '—'),
						diagnosticFact(_('Last Apply'), lastApply?.sequence),
						diagnosticFact(_('Apply result'), lastResult?.ok ? _('Succeeded') : (lastApply ? _('Failed') : '—'))
					])
				])
			]),
			...actions
		]),
		...desiredWarnings.map((issue) => E('p', { 'class': 'alert-message warning' }, _('Pending warning: %s · %s').format(issue.code, issue.message))),
		...savedWarnings.map((issue) => E('p', { 'class': 'alert-message warning' }, _('Saved warning: %s · %s').format(issue.code, issue.message))),
		lastResult?.ok === false ? E('p', { 'class': 'alert-message danger' }, _('The last Apply failed. Saved remains committed and Active remains the generation shown above.')) : '',
		E('p', { 'class': 'alert-message notice' }, _('Subscription inventory refreshes do not change Route or Rule selection and do not create pending Apply by themselves.'))
	]);
}

return view.extend({
	load: function() {
		return Promise.all([ uci.load('steer'), steer.overviewState(), steer.validate(), steer.diagnostics(), uci.changes() ]);
	},

	render: function(data) {
		let m, s, o;
		const lifecycle = data[1] || {};
		const status = lifecycle.active || {};
		const validation = data[2];
		const page = (window.location.pathname || '').split('/').pop();
		steer.loadStyle();
		if (page == 'overview' || page == 'steer')
			return renderLifecycleOverview(lifecycle);
		if (page == 'diagnostics')
			return renderDiagnostics(status, validation, data[3] || {}, data[4]);

		m = new form.Map('steer', _('Steer'));
		s = m.section(form.NamedSection, 'main', 'steer', _('General'));

		o = s.option(form.Flag, 'enabled', _('Enable Steer'));
		o.rmempty = false;
		o.description = _('A disabled configuration stops Steer and removes its runtime resources when applied.');

		o = s.option(form.ListValue, 'log_level', _('Log level'));
		uiSpec.log_levels.forEach((item) => o.value(item.value, item.label));
		o.default = 'warn';

		o = s.option(form.Value, 'dns_cache_capacity', _('Cache capacity'));
		o.datatype = 'range(1024,10000000)';
		o.placeholder = '4096';

		o = s.option(form.Flag, 'dns_cache_persist', _('Persistent cache'));
		o.default = '0';
		o.description = _('Persist the shared DNS cache in the Steer cache database.');

		o = s.option(form.Flag, 'dns_optimistic_cache', _('Optimistic cache'));
		o.default = '0';
		o.description = _('Serve recently expired DNS answers while refreshing them in the background.');

		o = s.option(form.Value, 'probe_direct', _('Direct connectivity probe URL'));
		o.datatype = 'url';
		o.rmempty = false;
		o.placeholder = 'https://www.example.com/';

		o = s.option(form.Value, 'probe_proxy', _('Proxy connectivity probe URL'));
		o.datatype = 'url';
		o.rmempty = false;
		o.placeholder = 'https://www.example.com/';

		o = s.option(form.Value, 'speedtest_proxy', _('Proxy speed-test URL'));
		o.datatype = 'url';
		o.rmempty = false;
		o.placeholder = 'https://speed.cloudflare.com/__down?bytes=1000000';

		s = m.section(form.NamedSection, 'bootstrap', 'bootstrap', _('Bootstrap DNS'));
		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		uiSpec.bootstrap_protocols.forEach((item) => o.value(item.value, item.label)); o.rmempty = false;
		o = s.option(form.Value, 'server', _('Server IP')); o.datatype = 'ipaddr'; o.rmempty = false;
		o = s.option(form.Value, 'server_port', _('Port')); o.datatype = 'port'; o.rmempty = false;
		o = s.option(form.ListValue, 'strategy', _('Address strategy'));
		uiSpec.bootstrap_strategies.forEach((item) => o.value(item.value, item.label));
		o.rmempty = false;

		return m.render();
	},

	handleSaveApply: function(ev, mode) { return steer.apply(this, ev, mode); }
});
