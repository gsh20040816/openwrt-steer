/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require view';
'require steer as steer';

const redactedValue = '[REDACTED]';

function secretKey(key) {
	return key == 'private_key' || /(^|_)(password|token)$/.test(key);
}

function redactSecrets(value) {
	if (Array.isArray(value))
		return value.map(redactSecrets);
	if (value == null || typeof(value) != 'object')
		return value;
	return Object.fromEntries(Object.entries(value).map(([ key, item ]) => [
		key,
		secretKey(key) && item != null && String(item) != '' ? redactedValue : redactSecrets(item)
	]));
}

return view.extend({
	load: function() {
		return steer.intentPreview(false);
	},

	render: function(preview) {
		steer.loadStyle();
		if (preview?.ok !== true) {
			return E('section', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Canonical Preview unavailable')),
				E('p', { 'class': 'alert-message danger' }, _('Unable to export the Canonical preview.'))
			]);
		}

		let current = preview;
		let revealed = false;
		const heading = E('h3');
		const notice = E('p');
		const document = E('pre', { 'class': 'steer-canonical-preview' });
		const reveal = E('button', {
			'type': 'button',
			'class': 'cbi-button cbi-button-action',
			'aria-pressed': 'false',
			'click': async function() {
				if (revealed) {
					current = Object.assign({}, current, {
						redacted: true,
						intent: redactSecrets(current.intent || {})
					});
					revealed = false;
					document.replaceChildren(JSON.stringify(current.intent, null, 2));
					reveal.setAttribute('aria-pressed', 'false');
					reveal.textContent = _('Reveal secrets temporarily');
					return;
				}

				reveal.disabled = true;
				const revealedPreview = await steer.intentPreview(true);
				reveal.disabled = false;
				if (revealedPreview?.ok !== true) {
					revealError.hidden = false;
					return;
				}
				revealError.hidden = true;
				current = revealedPreview;
				revealed = true;
				updateMetadata();
				document.replaceChildren(JSON.stringify(revealedPreview.intent || {}, null, 2));
				reveal.setAttribute('aria-pressed', 'true');
				reveal.textContent = _('Hide secrets');
			}
		}, _('Reveal secrets temporarily'));
		const revealError = E('p', { 'class': 'alert-message danger', 'hidden': '' }, _('Secret reveal failed.'));

		function updateMetadata() {
			const pending = current.source == 'pending';
			heading.replaceChildren(pending ? _('Pending candidate') : _('Committed snapshot'));
			notice.className = pending ? 'alert-message warning' : '';
			notice.replaceChildren(pending
				? _('This preview is the real pending UCI candidate that validation and Apply will receive.')
				: _('No pending Steer changes exist. This preview is the committed UCI snapshot.'));
		}

		updateMetadata();
		document.replaceChildren(JSON.stringify(redactSecrets(preview.intent || {}), null, 2));
		return E('section', { 'class': 'cbi-section' }, [
			E('h2', {}, _('Canonical Preview')),
			heading,
			notice,
			E('p', {}, _('Secrets are hidden by default. Reveal is temporary and resets when you leave this page.')),
			E('div', { 'class': 'right' }, reveal),
			revealError,
			document
		]);
	}
});
