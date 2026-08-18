/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require form';
'require uci';
'require view';
'require steer as steer';

function addSectionValues(option, sections) {
	sections.forEach((section) => option.value(section['.name'], section.name || section['.name']));
}

return view.extend({
	load: function() {
		return uci.load('steer');
	},

	render: function() {
		let m, s, o;
		const servers = uci.sections('steer', 'dns_server');

		m = new form.Map('steer', _('DNS'),
			_('A rule selects one DNS profile. Each profile owns one SmartDNS instance. Its upstream connections are router-local traffic and use the same ordinary Rules as every other local connection.'));

		s = m.section(form.GridSection, 'dns_profile', _('DNS profiles'));
		s.anonymous = true;
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add DNS profile');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};
		s.tab('general', _('Profile'));
		s.tab('cache', _('Cache'));

		o = s.taboption('general', form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.taboption('general', form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.taboption('general', form.Value, 'listen_port', _('Local port'));
		o.datatype = 'port';
		o.rmempty = false;
		o.modalonly = true;

		o = s.taboption('general', form.DynamicList, 'server', _('Upstream servers'));
		o.rmempty = false;
		o.modalonly = true;
		addSectionValues(o, servers);

		o = s.taboption('general', form.ListValue, 'response_mode', _('Response mode'));
		o.value('first-ping', _('First ping response'));
		o.value('fastest-ip', _('Fastest IP'));
		o.value('fastest-response', _('Fastest DNS response'));
		o.rmempty = false;
		o.editable = true;

		o = s.taboption('general', form.Value, 'speed_check_mode', _('Speed checks'));
		o.placeholder = 'none';
		o.modalonly = true;
		o.description = _('Examples: none, ping,tcp:443,tcp:80');

		o = s.taboption('general', form.ListValue, 'failure', _('If all upstreams fail'));
		o.value('block', _('Return failure'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.taboption('cache', form.Value, 'cache_size', _('Cache entries'));
		o.datatype = 'uinteger';
		o.modalonly = true;

		o = s.taboption('cache', form.Flag, 'cache_persist', _('Persistent cache'));
		o.default = '1';
		o.modalonly = true;

		o = s.taboption('cache', form.Flag, 'serve_expired', _('Serve expired records'));
		o.default = '1';
		o.modalonly = true;

		o = s.taboption('cache', form.Value, 'rr_ttl_min', _('Minimum TTL'));
		o.datatype = 'uinteger';
		o.modalonly = true;

		s = m.section(form.GridSection, 'dns_server', _('Upstream DNS servers'));
		s.anonymous = true;
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add upstream DNS server');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};

		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		o.value('udp', 'UDP');
		o.value('tcp', 'TCP');
		o.value('tls', 'DoT');
		o.value('https', 'DoH');
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.Value, 'server', _('Server'));
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.Value, 'server_port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.Value, 'tls_server_name', _('TLS server name'));
		o.modalonly = true;
		o.depends('protocol', 'tls');
		o.depends('protocol', 'https');

		o = s.option(form.Value, 'path', _('DoH path'));
		o.placeholder = '/dns-query';
		o.modalonly = true;
		o.depends('protocol', 'https');

		o = s.option(form.Flag, 'insecure', _('Skip certificate verification'));
		o.default = '0';
		o.modalonly = true;
		o.depends('protocol', 'tls');
		o.depends('protocol', 'https');

		return m.render();
	},

	handleSaveApply: function(ev, mode) {
		return steer.apply(this, ev, mode);
	}
});
