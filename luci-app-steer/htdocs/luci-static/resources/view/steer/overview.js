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

function reportScopeLabel(scope) {
	return {
		overview: _('Overview'),
		node: _('Node'), nodes: _('Node'),
		route: _('Route'), routes: _('Route')
	}[scope] || scope;
}

function reportKindLabel(kind) {
	return {
		direct: _('Direct'),
		proxy: _('Proxy'),
		speedtest: _('Download test'),
		connect: _('Connection test'),
		download: _('Download test')
	}[kind] || kind;
}

function diagnosticErrorText(error) {
	return {
		'probe timed out': _('Probe timed out'),
		'probe failed': _('Probe failed'),
		'Unable to read Steer logs.': _('Unable to read Steer logs.')
	}[String(error)] || error;
}

function diagnosticDetail(detail) {
	const value = String(detail || '');
	if (value == 'the published Active generation contains the expected port-53 capture artifacts')
		return _('System DNS capture is configured.');
	return detail;
}

function renderTestCard(kind, title, description, allowed) {
	const output = E('div', { 'class': 'steer-test-card__output' }, E('span', {}, _('Not tested')));
	const button = E('button', {
		'class': 'btn cbi-button-action',
		'disabled': allowed ? null : true,
		'title': allowed ? '' : _('You do not have permission to run Overview probes.'),
		'click': function(ev) {
			ev.preventDefault();
			if (!allowed) {
				ui.addNotification(_('Probe is not permitted'), E('p', {}, _('Your session does not have permission to run Overview probes.')), 'warning');
				return;
			}
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

function renderOverviewTests(allowed) {
	return E('div', { 'class': 'steer-test-grid' }, [
		renderTestCard('direct', _('Direct target'), _('Tests the direct target with the current running configuration.'), allowed),
		renderTestCard('proxy', _('Proxy target'), _('Tests the proxy target with the current running configuration.'), allowed),
		renderTestCard('speedtest', _('Download target'), _('Tests download speed with the current running configuration.'), allowed)
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
	const target = report.scope == 'overview' ? _('Overview') : reportScopeLabel(report.scope);
	return E('article', { 'class': 'cbi-section' }, [
		E('h4', {}, [ target, ' · ', reportKindLabel(report.kind), ' · ', stale ? _('Outdated') : (report.ok ? _('Succeeded') : _('Failed')) ]),
		E('dl', { 'class': 'steer-status__facts' }, [
			diagnosticFact(_('Tested at'), report.tested_at), diagnosticFact(_('URL'), result.url),
			diagnosticFact(_('Attempts'), result.attempts), diagnosticFact(_('Connect'), result.connect_milliseconds == null ? null : result.connect_milliseconds + ' ms'),
			diagnosticFact(_('TLS'), result.tls_milliseconds == null ? null : result.tls_milliseconds + ' ms'),
			diagnosticFact(_('TTFB'), result.first_byte_milliseconds == null ? null : result.first_byte_milliseconds + ' ms'),
			diagnosticFact(_('HTTP'), result.status), diagnosticFact(_('Bytes'), result.downloaded_bytes),
			diagnosticFact(_('Rate'), rate)
		]),
		(report.error || result.error) ? E('p', { 'class': 'alert-message danger' }, diagnosticErrorText(report.error || result.error)) : ''
	]);
}

function renderDiagnostics(status, validation, diagnostics, changes, permissions) {
	const pending = hasPendingSteerChanges(changes);
	const lastApply = status?.last_apply;
	const result = lastApply?.result;
	const dnsCapture = diagnostics?.dns_capture || {};
	return E([], [
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Connectivity targets')),
			E('p', {}, _('The tests use the current running configuration. Success only means the target was reachable at that time.')),
			renderOverviewTests(permissions?.overview_probe === true)
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('System DNS capture check')),
			E('dl', { 'class': 'steer-status__facts' }, [
				diagnosticFact(_('Configured'), dnsCapture.configured ? _('Yes') : _('No')),
				diagnosticFact(_('Result'), diagnosticDetail(dnsCapture.detail))
			])
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Recent connectivity reports')),
			...(diagnostics?.warnings || []).map((warning) => E('p', { 'class': 'alert-message warning' }, warning)),
			...((diagnostics?.reports || []).length ? diagnostics.reports.map((report) => renderProbeReport(report, diagnostics, pending)) : [ E('p', {}, _('No saved probe reports.')) ])
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Validation')),
			...((validation?.errors || []).map((issue) => E('p', { 'class': 'alert-message danger' }, steer.issueText(issue)))),
			...((validation?.warnings || []).map((issue) => E('p', { 'class': 'alert-message warning' }, steer.issueText(issue)))),
			validation?.ok ? E('p', {}, _('The saved configuration is valid.')) : ''
		]),
		E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Recent application result')),
			E('dl', { 'class': 'steer-status__facts' }, [
				diagnosticFact(_('Result'), result?.ok ? _('Succeeded') : (lastApply ? _('Failed') : '—')),
				diagnosticFact(_('Error'), result?.error)
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
	return _('Nodes %d · Routes %d · DNS %d · Proxies %d · Rules %d · Subscriptions %d').format(
		counts.nodes || 0, counts.routes || 0, counts.dns_profiles || 0, counts.local_proxies || 0, counts.rules || 0, counts.subscriptions || 0);
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
			if (result?.ok === false) throw result;
			actionState.replaceChildren(success);
			window.location.reload();
		}).catch((error) => {
			button.disabled = false;
			actionState.replaceChildren(error?.error_code ? steer.rpcErrorText(error) : _('Operation failed.'));
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
			E('h3', {}, _('Configuration status')),
			pending
				? E('p', { 'class': 'alert-message warning' }, _('The working copy has unsaved changes. The running configuration remains unchanged until they are saved and applied.'))
				: E('p', {}, _('The working copy is saved.')),
			E('div', { 'class': 'steer-test-grid' }, [
				E('article', { 'class': 'steer-test-card' }, [
					E('h4', {}, _('Working copy')),
					E('dl', { 'class': 'steer-status__facts' }, [
						diagnosticFact(_('Unsaved changes'), pending ? _('Yes') : _('No')),
						diagnosticFact(_('Enabled'), desired.enabled ? _('Enabled') : _('Disabled')),
						diagnosticFact(_('Objects'), lifecycleCounts(desired.counts)),
						diagnosticFact(_('Validation'), desired.validation?.ok ? _('Valid') : _('Invalid')),
						diagnosticFact(_('Warnings'), desiredWarnings.length)
					])
				]),
				E('article', { 'class': 'steer-test-card' }, [
					E('h4', {}, _('Saved configuration')),
					E('dl', { 'class': 'steer-status__facts' }, [
						diagnosticFact(_('Enabled'), saved.enabled ? _('Enabled') : _('Disabled')),
						diagnosticFact(_('Pending apply'), state?.pending_apply ? _('Yes') : _('No')),
						diagnosticFact(_('Objects'), lifecycleCounts(saved.counts)),
						diagnosticFact(_('Validation'), saved.validation?.ok ? _('Valid') : _('Invalid')),
						diagnosticFact(_('Warnings'), savedWarnings.length)
					])
				]),
				E('article', { 'class': 'steer-test-card' }, [
					E('h4', {}, _('Running configuration')),
					E('dl', { 'class': 'steer-status__facts' }, [
						diagnosticFact(_('Running'), active.generation ? _('Yes') : _('No')),
						diagnosticFact(_('Status'), active.generation ? (active.healthy ? _('Normal') : _('Abnormal')) : _('Stopped')),
						diagnosticFact(_('Apply result'), lastResult?.ok ? _('Succeeded') : (lastApply ? _('Failed') : '—'))
					])
				])
			]),
			...actions
		]),
		...desiredWarnings.map((issue) => E('p', { 'class': 'alert-message warning' }, _('Working copy warning: %s').format(steer.issueText(issue)))),
		...savedWarnings.map((issue) => E('p', { 'class': 'alert-message warning' }, _('Saved configuration warning: %s').format(steer.issueText(issue)))),
		lastResult?.ok === false ? E('p', { 'class': 'alert-message danger' }, _('The last application failed. The saved configuration is still available; check Diagnostics for details.')) : ''
	]);
}

return view.extend({
	load: function() {
		return Promise.all([
			uci.load('steer'), steer.overviewState(), steer.validate(), steer.diagnostics(), uci.changes(),
			steer.permissions([ 'overview_probe' ])
		]);
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
			return renderDiagnostics(status, validation, data[3] || {}, data[4], data[5] || {});

		m = new form.Map('steer', _('Steer'));
		s = m.section(form.NamedSection, 'main', 'steer', _('General'));

		o = s.option(form.Flag, 'enabled', _('Enable Steer'));
		o.rmempty = false;
		o.description = _('A disabled configuration stops Steer and removes its runtime resources when applied.');

		o = s.option(form.ListValue, 'log_level', _('Log level'));
		uiSpec.log_levels.forEach((item) => o.value(item.value, steer.uiSpecLabel(item.label)));
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
		o.validate = function(_sectionId, value) { return steer.validateInput('probe_url', value); };
		o.rmempty = false;
		o.placeholder = 'https://www.example.com/';

		o = s.option(form.Value, 'probe_proxy', _('Proxy connectivity probe URL'));
		o.validate = function(_sectionId, value) { return steer.validateInput('probe_url', value); };
		o.rmempty = false;
		o.placeholder = 'https://www.example.com/';

		o = s.option(form.Value, 'speedtest_proxy', _('Proxy speed-test URL'));
		o.validate = function(_sectionId, value) { return steer.validateInput('probe_url', value); };
		o.rmempty = false;
		o.placeholder = 'https://speed.cloudflare.com/__down?bytes=1000000';

		s = m.section(form.NamedSection, 'bootstrap', 'bootstrap', _('Bootstrap DNS'));
		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		uiSpec.bootstrap_protocols.forEach((item) => o.value(item.value, steer.uiSpecLabel(item.label))); o.rmempty = false;
		o = s.option(form.Value, 'server', _('Server IP')); o.datatype = 'ipaddr'; o.rmempty = false;
		o.description = _('Server IP used to resolve encrypted DNS upstream domains.');
		o = s.option(form.Value, 'server_port', _('Port')); o.datatype = 'port'; o.rmempty = false;
		o = s.option(form.ListValue, 'strategy', _('Address strategy'));
		uiSpec.bootstrap_strategies.forEach((item) => o.value(item.value, steer.uiSpecLabel(item.label)));
		o.rmempty = false;

		return m.render();
	},

	handleSaveApply: function(ev, mode) { return steer.apply(this, ev, mode); }
});
