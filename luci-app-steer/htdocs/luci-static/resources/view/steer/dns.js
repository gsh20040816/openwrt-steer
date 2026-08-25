/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require form';
'require uci';
'require view';
'require steer as steer';
'require steer.ui-spec as uiSpec';

return view.extend({
	load: function() { return uci.load('steer'); },

	render: function() {
		let m, s, o;
		steer.loadStyle();
		m = new form.Map('steer', _('DNS profiles'));
		s = m.section(form.GridSection, 'dns_profile', _('DNS profiles'));
		steer.configureNamedSection(s);
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add DNS profile');
		s.sectiontitle = function(sectionId) { return uci.get('steer', sectionId, 'name') || sectionId; };
		s.tab('general', _('Upstream'));
		s.tab('tls', _('TLS'));

		o = s.taboption('general', form.Flag, 'enabled', _('Enabled')); o.default = '1'; o.editable = true;
		o = s.taboption('general', form.Value, 'name', _('Name')); o.rmempty = false; o.modalonly = true;
		o = s.taboption('general', form.ListValue, 'protocol', _('Protocol'));
		uiSpec.dns_protocols.forEach((item) => o.value(item.value, item.label));
		o.rmempty = false; o.editable = true;
		o = s.taboption('general', form.Value, 'server', _('Server')); o.rmempty = false; o.editable = true;
		o = s.taboption('general', form.Value, 'server_port', _('Port')); o.datatype = 'port'; o.rmempty = false; o.modalonly = true;
		o = s.taboption('general', form.Value, 'path', _('HTTP path')); o.placeholder = '/dns-query'; o.modalonly = true; o.depends('protocol', 'https'); o.depends('protocol', 'h3');

		o = s.taboption('tls', form.Value, 'tls_server_name', _('TLS server name')); o.modalonly = true;
		[ 'tls', 'https', 'quic', 'h3' ].forEach((protocol) => o.depends('protocol', protocol));
		o = s.taboption('tls', form.Flag, 'insecure', _('Skip certificate verification')); o.default = '0'; o.modalonly = true;
		[ 'tls', 'https', 'quic', 'h3' ].forEach((protocol) => o.depends('protocol', protocol));

		return m.render();
	},

	handleSaveApply: function(ev, mode) { return steer.apply(this, ev, mode); }
});
