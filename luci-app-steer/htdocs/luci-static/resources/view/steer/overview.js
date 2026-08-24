/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require form';
'require uci';
'require ui';
'require view';
'require steer as steer';

function testResult(report, kind) {
	const result = report?.results?.[0];
	if (!report?.ok || !result?.ok) {
		return E('div', { 'class': 'steer-test-card__result is-error' }, [
			E('strong', {}, _('Failed')),
			E('small', {}, result?.error || report?.error || _('No test result was returned.'))
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
		steer.loadStyle();

		m = new form.Map('steer', _('Steer'));
		s = m.section(form.NamedSection, 'main', 'steer', _('Traffic steering'));
		s.tab('general', _('General'));
		s.tab('dns', _('DNS cache'));
		s.tab('probes', _('Manual probes'));

		o = s.taboption('general', form.Flag, 'enabled', _('Enable Steer'));
		o.rmempty = false;
		o.description = _('A disabled configuration stops Steer and removes its runtime resources when applied.');

		o = s.taboption('general', form.ListValue, 'log_level', _('Log level'));
		[ 'error', 'warn', 'info', 'debug' ].forEach((level) => o.value(level, level));
		o.default = 'warn';

		o = s.taboption('dns', form.Value, 'dns_cache_capacity', _('Cache capacity'));
		o.datatype = 'range(1024,10000000)';
		o.placeholder = '4096';

		o = s.taboption('dns', form.Flag, 'dns_cache_persist', _('Persistent cache'));
		o.default = '0';
		o.description = _('Persist the shared DNS cache in the Steer cache database.');

		o = s.taboption('dns', form.Flag, 'dns_optimistic_cache', _('Optimistic cache'));
		o.default = '0';
		o.description = _('Serve recently expired DNS answers while refreshing them in the background.');

		o = s.taboption('probes', form.Value, 'probe_direct', _('Direct connectivity probe URL'));
		o.datatype = 'url';
		o.rmempty = false;
		o.placeholder = 'https://www.example.com/';

		o = s.taboption('probes', form.Value, 'probe_proxy', _('Proxy connectivity probe URL'));
		o.datatype = 'url';
		o.rmempty = false;
		o.placeholder = 'https://www.example.com/';

		o = s.taboption('probes', form.Value, 'speedtest_proxy', _('Proxy speed-test URL'));
		o.datatype = 'url';
		o.rmempty = false;
		o.placeholder = 'https://speed.cloudflare.com/__down?bytes=1000000';

		s = m.section(form.NamedSection, 'bootstrap', 'bootstrap', _('Bootstrap DNS'));
		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		o.value('udp', 'UDP'); o.value('tcp', 'TCP'); o.rmempty = false;
		o = s.option(form.Value, 'server', _('Server IP')); o.datatype = 'ipaddr'; o.rmempty = false;
		o = s.option(form.Value, 'server_port', _('Port')); o.datatype = 'port'; o.rmempty = false;
		o = s.option(form.ListValue, 'strategy', _('Address strategy'));
		[ 'prefer_ipv4', 'prefer_ipv6', 'ipv4_only', 'ipv6_only' ].forEach((value) => o.value(value, value));
		o.rmempty = false;

		s = m.section(form.GridSection, 'subscription', _('Node subscriptions'));
		s.anonymous = false;
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add subscription');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || uci.get('steer', sectionId, 'url') || _('Unnamed');
		};
		s.handleAdd = function(ev, sectionId) {
			if (!/^[a-z][a-z0-9_]{0,31}$/.test(sectionId)) {
				ui.addNotification(_('Invalid subscription ID'), E('p', {}, _('Use 1–32 lowercase characters beginning with a letter.')), 'danger');
				return;
			}
			this.map.data.add(this.uciconfig || this.map.config, this.sectiontype, sectionId);
			return this.map.save(null, true);
		};
		o = s.option(form.Flag, 'enabled', _('Enabled')); o.default = '1'; o.editable = true;
		o = s.option(form.Value, 'name', _('Name')); o.rmempty = false; o.modalonly = true;
		o = s.option(form.Value, 'url', _('Subscription URL')); o.datatype = 'url'; o.rmempty = false; o.editable = true;
		o = s.option(form.Value, 'update_interval', 'Update interval'); o.placeholder = '6h'; o.modalonly = true;

		return m.render().then((formNode) => E([], [
			steer.renderStatus(status, validation, uci.get('steer', 'main', 'enabled') == '1'),
			renderOverviewTests(), formNode
		]));
	},

	handleSaveApply: function(ev, mode) { return steer.apply(this, ev, mode); }
});
