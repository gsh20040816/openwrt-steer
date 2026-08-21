/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require form';
'require uci';
'require ui';
'require view';
'require steer as steer';

return view.extend({
	load: function() {
		return Promise.all([ uci.load('steer'), steer.status() ]);
	},

	render: function(data) {
		let m, s, o;
		const status = data[1];
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
		o.description = _('Reserved for sing-box 1.14; enabling it on M1 is rejected.');

		o = s.taboption('dns', form.Flag, 'dns_optimistic_cache', _('Optimistic cache'));
		o.default = '0';
		o.description = _('Reserved for sing-box 1.14; enabling it on M1 is rejected.');

		o = s.taboption('probes', form.DynamicList, 'probe_direct', _('Direct connectivity probe URLs'));
		o.datatype = 'url';
		o.placeholder = 'https://www.example.com/';

		o = s.taboption('probes', form.DynamicList, 'probe_proxy', _('Proxy connectivity probe URLs'));
		o.datatype = 'url';
		o.placeholder = 'https://www.example.com/';

		o = s.taboption('probes', form.DynamicList, 'speedtest_proxy', _('Proxy speed-test URLs'));
		o.datatype = 'url';
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
		o = s.option(form.Value, 'url', 'HTTPS subscription URL'); o.datatype = 'url'; o.rmempty = false; o.editable = true;
		o = s.option(form.Value, 'update_interval', 'Update interval'); o.placeholder = '6h'; o.modalonly = true;

		return m.render().then((formNode) => E([], [ steer.renderStatus(status), formNode ]));
	},

	handleSaveApply: function(ev, mode) { return steer.apply(this, ev, mode); }
});
