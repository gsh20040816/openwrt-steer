/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require form';
'require uci';
'require view';
'require steer as steer';

return view.extend({
	load: function() { return uci.load('steer'); },

	render: function() {
		let m, s, o;
		m = new form.Map('steer', _('DNS profiles'), _('Each profile owns exactly one upstream. Every referenced (DNS profile, route) pair becomes an independent sing-box DNS transport and cache path.'));
		s = m.section(form.GridSection, 'dns_profile', _('DNS profiles'));
		s.anonymous = true;
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add DNS profile');
		s.sectiontitle = function(sectionId) { return uci.get('steer', sectionId, 'name') || sectionId; };
		s.tab('general', _('Upstream'));
		s.tab('tls', _('TLS'));
		s.tab('cache', _('Cache'));

		o = s.taboption('general', form.Flag, 'enabled', _('Enabled')); o.default = '1'; o.editable = true;
		o = s.taboption('general', form.Value, 'name', _('Name')); o.rmempty = false; o.modalonly = true;
		o = s.taboption('general', form.ListValue, 'protocol', _('Protocol'));
		o.value('udp', 'UDP'); o.value('tcp', 'TCP'); o.value('tls', 'DoT'); o.value('https', 'DoH'); o.value('quic', 'DoQ'); o.value('h3', 'DoH3');
		o.rmempty = false; o.editable = true;
		o = s.taboption('general', form.Value, 'server', _('Server')); o.rmempty = false; o.editable = true;
		o = s.taboption('general', form.Value, 'server_port', _('Port')); o.datatype = 'port'; o.rmempty = false; o.modalonly = true;
		o = s.taboption('general', form.ListValue, 'strategy', _('Address strategy'));
		[ 'prefer_ipv4', 'prefer_ipv6', 'ipv4_only', 'ipv6_only' ].forEach((value) => o.value(value, value));
		o.default = 'prefer_ipv4'; o.rmempty = false; o.modalonly = true;
		o = s.taboption('general', form.Value, 'path', _('HTTP path')); o.placeholder = '/dns-query'; o.modalonly = true; o.depends('protocol', 'https'); o.depends('protocol', 'h3');

		o = s.taboption('tls', form.Value, 'tls_server_name', _('TLS server name')); o.modalonly = true;
		[ 'tls', 'https', 'quic', 'h3' ].forEach((protocol) => o.depends('protocol', protocol));
		o = s.taboption('tls', form.Flag, 'insecure', _('Skip certificate verification')); o.default = '0'; o.modalonly = true;
		[ 'tls', 'https', 'quic', 'h3' ].forEach((protocol) => o.depends('protocol', protocol));

		o = s.taboption('cache', form.Flag, 'cache_persist', _('Persistent cache')); o.default = '0'; o.modalonly = true; o.description = _('Reserved for sing-box 1.14; M1 rejects it.');
		o = s.taboption('cache', form.Flag, 'optimistic_cache', _('Optimistic cache')); o.default = '0'; o.modalonly = true; o.description = _('Reserved for sing-box 1.14; M1 rejects it.');
		return m.render();
	},

	handleSaveApply: function(ev, mode) { return steer.apply(this, ev, mode); }
});
