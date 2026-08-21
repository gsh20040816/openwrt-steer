/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require form';
'require uci';
'require view';
'require steer as steer';

function renderPlan(result) {
	const plan = result?.plan || result;
	const diff = result?.diff;
	if (!plan?.schema_version)
		return E('div', { 'class': 'alert-message warning' }, result?.error || _('Execution plan is unavailable.'));
	let summary = [
		[ _('Schema'), String(plan.schema_version) ],
		[ _('Ingress scope'), _('All interfaces') ],
		[ _('TUN interface'), plan.resources?.tun_interface || '-' ],
		[ _('DNS paths'), String((plan.dns_paths || []).length) ],
		[ _('Geo rule sets'), String((plan.geo_rule_sets || []).length) ]
	];
	if (diff)
		summary.push([ _('Candidate changes'), diff.changed ? _('%d added, %d modified, %d removed').format(diff.added.length, diff.modified.length, diff.removed.length) : _('No semantic changes') ]);
	return E('section', { 'class': 'cbi-section' }, [
		E('h3', {}, _('Execution plan')),
		E('dl', { 'class': 'steer-status__facts' }, summary.map((fact) => E('div', {}, [ E('dt', {}, fact[0]), E('dd', {}, fact[1]) ]))),
		E('details', {}, [ E('summary', {}, _('Show complete plan')), E('pre', {}, JSON.stringify(result, null, 2)) ])
	]);
}

return view.extend({
	load: function() {
		return Promise.all([ uci.load('steer'), steer.status(), steer.plan() ]);
	},

	render: function(data) {
		let m, s, o;
		const status = data[1];
		steer.loadStyle();

		m = new form.Map('steer', _('Steer'), _('Compile explicit routing and DNS intent into one verified sing-box execution plan.'));
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
		o.description = _('Reserved for sing-box 1.14; enabling it on M1 is rejected.');

		o = s.taboption('dns', form.Flag, 'dns_optimistic_cache', _('Optimistic cache'));
		o.default = '0';
		o.description = _('Reserved for sing-box 1.14; enabling it on M1 is rejected.');

		o = s.taboption('probes', form.DynamicList, 'probe_url', _('HTTPS probe URLs'));
		o.datatype = 'url';
		o.placeholder = 'https://www.example.com/';
		o.description = _('The explicit “steer probe” diagnostic reports valid TLS 2xx or 3xx responses. Probes never gate Apply.');

		s = m.section(form.NamedSection, 'bootstrap', 'bootstrap', _('Bootstrap DNS'));
		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		o.value('udp', 'UDP'); o.value('tcp', 'TCP'); o.rmempty = false;
		o = s.option(form.Value, 'server', _('Server IP')); o.datatype = 'ipaddr'; o.rmempty = false;
		o = s.option(form.Value, 'server_port', _('Port')); o.datatype = 'port'; o.rmempty = false;
		o = s.option(form.ListValue, 'strategy', _('Address strategy'));
		[ 'prefer_ipv4', 'prefer_ipv6', 'ipv4_only', 'ipv6_only' ].forEach((value) => o.value(value, value));
		o.rmempty = false;

		return m.render().then((formNode) => E([], [ steer.renderStatus(status), renderPlan(data[2]), formNode ]));
	},

	handleSaveApply: function(ev, mode) { return steer.apply(this, ev, mode); }
});
