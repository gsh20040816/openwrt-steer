// SPDX-License-Identifier: GPL-3.0-or-later

const token = document.querySelector('#token');
token.value = sessionStorage.getItem('steer-token') || '';
const view = document.querySelector('#view');
const api = async (path, options = {}) => {
  options.headers = {...options.headers, Authorization: `Bearer ${token.value}`};
  const response = await fetch(path, options);
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || response.statusText);
  return {response, data};
};
const card = (title) => { const section = document.createElement('section'); section.className = 'card'; const heading = document.createElement('h2'); heading.textContent = title; section.append(heading); return section; };
const text = (parent, value) => { const pre = document.createElement('pre'); pre.textContent = typeof value === 'string' ? value : JSON.stringify(value, null, 2); parent.append(pre); };
const showCollection = async (name, key, title) => { const {data} = await api('/api/v1/config'); const section = card(title); const items = data.intent[key] || []; const table = document.createElement('table'); table.innerHTML = '<thead><tr><th>ID</th><th>名称</th><th>状态</th><th>详情</th></tr></thead>'; const body = document.createElement('tbody'); items.forEach(item => { const row = document.createElement('tr'); [item.id || '', item.name || '', item.enabled === false ? 'disabled' : 'enabled'].forEach(value => { const cell = document.createElement('td'); cell.textContent = value; row.append(cell); }); const detail = document.createElement('td'); const pre = document.createElement('pre'); pre.textContent = JSON.stringify(item, null, 2); detail.append(pre); row.append(detail); body.append(row); }); table.append(body); section.append(table); const hint = document.createElement('p'); hint.textContent = '对象编辑保持 Canonical JSON 严格校验；打开“配置”页进行保存或 Save & Apply。'; section.append(hint); view.replaceChildren(section); };
const show = async (name) => {
  try {
    if (name === 'overview') {
      const {data} = await api('/api/v1/overview'); const section = card('概览'); const state = document.createElement('p'); state.className = data.status.healthy ? 'ok' : 'bad'; state.textContent = `运行状态：${data.status.healthy ? 'healthy' : 'unhealthy/disabled'}`; section.append(state); const saved = document.createElement('p'); saved.textContent = `保存配置：${data.saved_valid ? '合法' : '非法'}；revision：${data.saved_revision || '不可用'}`; section.append(saved); text(section, data.validation); view.replaceChildren(section);
    } else if (['nodes', 'routes', 'dns', 'rules', 'proxies'].includes(name)) {
      const keys = {nodes: ['nodes', '节点'], routes: ['routes', '路由'], dns: ['dns_profiles', 'DNS Profile'], rules: ['rules', '规则'], proxies: ['local_proxies', '本地代理']}; await showCollection(name, keys[name][0], keys[name][1]);
    } else if (name === 'config') {
      const {data, response} = await api('/api/v1/config'); let etag = response.headers.get('ETag') || data.revision; const section = card('配置'); const revision = document.createElement('p'); revision.textContent = `Revision: ${etag}`; section.append(revision); const editor = document.createElement('textarea'); editor.value = JSON.stringify(data.intent, null, 2); section.append(editor); const actions = document.createElement('p'); const result = document.createElement('pre'); const save = async (apply) => { try { const body = JSON.stringify({intent: JSON.parse(editor.value), apply}); const response = await fetch('/api/v1/config', {method: 'PUT', headers: {Authorization: `Bearer ${token.value}`, 'Content-Type': 'application/json', 'If-Match': etag}, body}); const responseData = await response.json(); if (!response.ok) throw new Error(responseData.error || response.statusText); etag = response.headers.get('ETag') || responseData.revision || etag; revision.textContent = `Revision: ${etag}`; result.textContent = JSON.stringify(responseData, null, 2); } catch (error) { result.textContent = error.message; } }; [['保存', false], ['保存并 Apply', true]].forEach(([label, apply]) => { const button = document.createElement('button'); button.textContent = label; button.onclick = () => save(apply); actions.append(button, ' '); }); const applyButton = document.createElement('button'); applyButton.textContent = 'Apply 已保存配置'; applyButton.onclick = async () => { try { const {data} = await api('/api/v1/apply', {method: 'POST'}); result.textContent = JSON.stringify(data, null, 2); } catch (error) { result.textContent = error.message; } }; actions.append(applyButton); section.append(actions, result); view.replaceChildren(section);
    } else if (name === 'subscriptions') {
      const {data} = await api('/api/v1/subscriptions'); const section = card('订阅'); data.subscriptions.forEach(subscription => { const row = document.createElement('p'); row.textContent = `${subscription.name || subscription.id}: ${subscription.node_count} nodes, skipped ${subscription.skipped}`; const button = document.createElement('button'); button.textContent = '立即更新'; button.onclick = async () => { try { const result = await api(`/api/v1/subscriptions/${encodeURIComponent(subscription.id)}/update`, {method: 'POST'}); text(section, result.data); } catch (error) { text(section, error.message); } }; row.append(' ', button); section.append(row); }); view.replaceChildren(section);
    } else if (name === 'diagnostics') {
      const section = card('诊断'); ['direct', 'proxy', 'speedtest'].forEach(kind => { const button = document.createElement('button'); button.textContent = `${kind} probe`; button.onclick = async () => { try { const {data} = await api('/api/v1/probes/overview', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({kind, download: kind === 'speedtest'})}); text(section, data); } catch (error) { text(section, error.message); } }; section.append(button, ' '); }); const note = document.createElement('p'); note.textContent = '裸节点/路由测速使用独立临时核心，并显式绕过当前 Steer 数据面。VM/Docker 公网流量由主 TUN 数据面接管。'; section.append(note); view.replaceChildren(section);
    }
  } catch (error) { const section = card('错误'); section.className += ' bad'; text(section, error.message); view.replaceChildren(section); }
};
document.querySelector('#connect').onclick = () => { sessionStorage.setItem('steer-token', token.value); show('overview'); };
document.querySelectorAll('nav button').forEach(button => button.onclick = () => show(button.dataset.view));
show('overview');
