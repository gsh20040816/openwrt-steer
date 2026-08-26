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
		renderTestCard('direct', _('Direct test'), _('Requests the configured direct URL through the current running rules.')),
		renderTestCard('proxy', _('Proxy test'), _('Requests the configured proxy URL through the current running rules.')),
		renderTestCard('speedtest', _('Proxy speed test'), _('Downloads the configured speed-test URL through the current running rules.'))
	]);
}

return view.extend({
	load: function() {
		return Promise.all([ uci.load('steer'), steer.status(), steer.validate() ]);
	},

	render: function(data) {
		let m, s, o;
		const status = data[1];
		const validation = data[2];
		const page = (window.location.pathname || '').split('/').pop();
		steer.loadStyle();
		if (page == 'overview' || page == 'steer')
			return E([], [ steer.renderStatus(status, validation, uci.get('steer', 'main', 'enabled') == '1') ]);
		if (page == 'diagnostics')
			return renderOverviewTests();

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
