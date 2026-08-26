#!/usr/bin/env python3
"""Verify generated UI specs and the shared native information architecture."""

import json
from pathlib import Path
import subprocess


ROOT = Path(__file__).resolve().parents[1]


def require(content: str, fragment: str, owner: str) -> None:
    if fragment not in content:
        raise SystemExit(f"check-ui-contract: {owner} does not consume {fragment}")


subprocess.run(
    ["go", "run", "./cmd/steer-ui-spec", "--root", "..", "--check"],
    cwd=ROOT / "go",
    check=True,
)

contract = json.loads((ROOT / "ui/steer-ui-spec.json").read_text())
local_proxy_fixtures = json.loads(
    (ROOT / "ui/local-proxy-listen-fixtures.json").read_text()
)
if local_proxy_fixtures.get("schema_version") != 1:
    raise SystemExit("check-ui-contract: invalid local proxy fixture schema")
fixture_by_listen = {
    item["listen"]: item for item in local_proxy_fixtures.get("cases", [])
}
for listen, classification in {
    "127.0.0.1": "loopback", "::1": "loopback",
    "0.0.0.0": "non_loopback", "::": "non_loopback",
    "192.168.50.1": "non_loopback", "fd12:3456:789a::1": "non_loopback",
    "router.lan": "invalid",
}.items():
    if fixture_by_listen.get(listen, {}).get("classification") != classification:
        raise SystemExit(f"check-ui-contract: missing local proxy fixture {listen!r}")
subscription_status_fixtures = json.loads(
    (ROOT / "ui/subscription-status-fixtures.json").read_text()
)
probe_diagnostics_fixtures = json.loads(
    (ROOT / "ui/probe-diagnostics-fixtures.json").read_text()
)
state_lifecycle_fixtures = json.loads(
    (ROOT / "ui/state-lifecycle-fixtures.json").read_text()
)
if state_lifecycle_fixtures.get("schema_version") != 1:
    raise SystemExit("check-ui-contract: invalid state lifecycle fixture schema")
if [item.get("name") for item in state_lifecycle_fixtures.get("cases", [])] != [
    "fresh", "pending-disable", "failed-apply", "active"
]:
    raise SystemExit("check-ui-contract: state lifecycle fixture drift")
if probe_diagnostics_fixtures.get("schema_version") != 1:
    raise SystemExit("check-ui-contract: invalid probe diagnostics fixture schema")
probe_reports = probe_diagnostics_fixtures.get("diagnostics", {}).get("reports", [])
if [report.get("scope") for report in probe_reports] != ["overview", "nodes", "routes"]:
    raise SystemExit("check-ui-contract: probe diagnostics fixture scope drift")
for collection in ("nodes", "routes", "subscriptions"):
    enabled_values = [item.get("enabled") for item in probe_diagnostics_fixtures.get("objects", {}).get(collection, [])]
    if enabled_values != [True, False]:
        raise SystemExit(f"check-ui-contract: {collection} enabled/disabled probe fixture drift")
if subscription_status_fixtures.get("schema_version") != 1:
    raise SystemExit("check-ui-contract: invalid subscription status fixture schema")
subscription_cases = {item.get("name"): item.get("status", {}) for item in subscription_status_fixtures.get("cases", [])}
required_subscription_cases = {
    "never-fetched", "success", "success-with-skipped",
    "failed-after-success-with-partial-stale-block", "disabled",
}
if set(subscription_cases) != required_subscription_cases:
    raise SystemExit(f"check-ui-contract: subscription lifecycle fixture drift: {set(subscription_cases)!r}")
for name, status in subscription_cases.items():
    for field in ("never_fetched", "last_success", "last_failure", "node_count", "current", "added", "skipped", "stale"):
        if field not in status:
            raise SystemExit(f"check-ui-contract: fixture {name!r} lacks {field!r}")
expected_navigation = [
    ("status", ["overview"]),
    ("configuration", ["general", "nodes", "routes", "dns", "proxies", "rules"]),
    ("services", ["subscriptions", "diagnostics", "system"]),
    ("advanced", ["advanced"]),
]
actual_navigation = [
    (group["key"], [item["key"] for item in group["items"]])
    for group in contract["navigation"]
]
if actual_navigation != expected_navigation:
    raise SystemExit(f"check-ui-contract: navigation drift: {actual_navigation!r}")

luci_menu = json.loads(
    (ROOT / "luci-app-steer/root/usr/share/luci/menu.d/luci-app-steer.json").read_text()
)
luci_root = "admin/services/steer"
expected_luci_items = [
    item for _, items in expected_navigation for item in items
]
luci_children = sorted(
    (
        (entry["order"], path.rsplit("/", 1)[-1])
        for path, entry in luci_menu.items()
        if path.startswith(luci_root + "/")
        and "/" not in path[len(luci_root) + 1:]
    ),
    key=lambda item: item[0],
)
actual_luci_items = [key for _, key in luci_children]
if actual_luci_items != expected_luci_items:
    raise SystemExit(
        f"check-ui-contract: LuCI flat navigation drift: {actual_luci_items!r}"
    )
for group_key, _ in expected_navigation:
    if group_key not in expected_luci_items and f"{luci_root}/{group_key}" in luci_menu:
        raise SystemExit(
            f"check-ui-contract: LuCI restored redundant group menu {group_key!r}"
        )

node_types = {item["value"] for item in contract["node_types"]}
if node_types != {
    "socks", "http", "shadowsocks", "vmess", "vless", "trojan", "hysteria",
    "hysteria2", "shadowtls", "tuic", "anytls", "naive", "ssh", "tor",
}:
    raise SystemExit("check-ui-contract: node protocol matrix is incomplete")
route_kind_labels = {item["value"]: item["label"] for item in contract["route_kinds"]}
if route_kind_labels.get("block") != "Reject":
    raise SystemExit("check-ui-contract: deprecated Block label returned instead of Reject")
if contract.get("subscription_update_interval_default") != "6h":
    raise SystemExit("check-ui-contract: shared subscription interval default drifted")

expected_dns_protocols = {
    "udp": ([], [], 53),
    "tcp": ([], [], 53),
    "tls": (["tls_server_name", "insecure"], ["tls_server_name"], 853),
    "https": (["tls_server_name", "path", "insecure"], ["tls_server_name"], 443),
    "quic": (["tls_server_name", "insecure"], ["tls_server_name"], 853),
    "h3": (["tls_server_name", "path", "insecure"], ["tls_server_name"], 443),
}
actual_dns_protocols = {
    item["value"]: (item["fields"], item["required_fields"], item["default_port"])
    for item in contract["dns_protocols"]
}
if actual_dns_protocols != expected_dns_protocols:
    raise SystemExit(f"check-ui-contract: DNS protocol field matrix drift: {actual_dns_protocols!r}")

linux_ui = (ROOT / "go/cmd/steer-linux/web/js/ui.js").read_text()
linux_web_api = (ROOT / "go/cmd/steer-linux/web_api.go").read_text()
linux_lib = (ROOT / "go/cmd/steer-linux/web/js/lib.js").read_text()
linux_nodes = (ROOT / "go/cmd/steer-linux/web/js/views/nodes.js").read_text()
linux_routes = (ROOT / "go/cmd/steer-linux/web/js/views/routes.js").read_text()
linux_subscriptions = (ROOT / "go/cmd/steer-linux/web/js/views/subscriptions.js").read_text()
linux_diagnostics = (ROOT / "go/cmd/steer-linux/web/js/views/diagnostics.js").read_text()
linux_dns = (ROOT / "go/cmd/steer-linux/web/js/views/dns.js").read_text()
linux_proxies = (ROOT / "go/cmd/steer-linux/web/js/views/proxies.js").read_text()
luci_nodes = (ROOT / "luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js").read_text()
luci_overview = (ROOT / "luci-app-steer/htdocs/luci-static/resources/view/steer/overview.js").read_text()
luci_helper = (ROOT / "luci-app-steer/htdocs/luci-static/resources/steer.js").read_text()
luci_dns = (ROOT / "luci-app-steer/htdocs/luci-static/resources/view/steer/dns.js").read_text()
luci_proxies = (ROOT / "luci-app-steer/htdocs/luci-static/resources/view/steer/local-proxies.js").read_text()
mac_content = (ROOT / "macos/SteerApp/ContentView.swift").read_text()
mac_state = (ROOT / "macos/SteerApp/AppState.swift").read_text()
mac_editors = (ROOT / "macos/SteerApp/DraftEditors.swift").read_text()
mac_ui_spec = (ROOT / "macos/SteerApp/UISpec.swift").read_text()
openwrt_cli = (ROOT / "go/cmd/steer-openwrt/main.go").read_text()
mac_control = (ROOT / "go/cmd/steer-macos/control.go").read_text()

require(linux_ui, "S.uiSpec.navigation", "Linux navigation")
require(linux_nodes, "S.uiSpec.node_fields", "Linux node form")
require(luci_nodes, "steer.ui-spec", "LuCI node form")
require(luci_nodes, "uiSpec.node_types", "LuCI protocol picker")
require(luci_nodes, "uiSpec.node_fields", "LuCI field matrix")
require(luci_nodes, "addGeneratedNodeField", "LuCI generated controls")
require(mac_content, "SteerUISpec.contract.navigation", "macOS navigation")
require(mac_editors, "SharedNodeDraftForm", "macOS node form")
require(mac_editors, "SteerUISpec.nodeFields", "macOS field matrix")
require(linux_subscriptions, "S.uiSpec.subscription_update_interval_default", "Linux subscription default")
require(linux_subscriptions, "update_interval: defaultInterval", "Linux subscription creation")
require(luci_nodes, "uiSpec.subscription_update_interval_default", "LuCI subscription default")
require(luci_nodes, "update_interval: uiSpec.subscription_update_interval_default", "LuCI subscription creation")
require(mac_state, "SteerUISpec.contract.subscriptionUpdateIntervalDefault", "macOS subscription default")
require(mac_state, '"update_interval": .string(SteerUISpec.contract.subscriptionUpdateIntervalDefault)', "macOS subscription creation")
require(linux_dns, "item.fields", "Linux DNS field matrix")
require(linux_dns, "next.default_port", "Linux DNS port matrix")
require(luci_dns, "protocol.fields", "LuCI DNS field matrix")
require(luci_dns, "next.default_port", "LuCI DNS port matrix")
require(mac_ui_spec, "UIDNSProtocolSpec", "macOS DNS field matrix")
require(mac_ui_spec, "next.defaultPort", "macOS DNS port matrix")
require(mac_ui_spec, "currentPort == $0.defaultPort", "macOS DNS custom-port preservation")
require(mac_editors, "protocolSpec?.fields.contains", "macOS DNS field visibility")
require(linux_ui, "classifyLocalProxyListen", "Linux local proxy address classification")
require(linux_proxies, "PROTOCOL_LABEL", "Linux shared local proxy labels")
require(linux_proxies, "保持已保存密码", "Linux explicit password retention")
require(linux_proxies, "替换密码", "Linux explicit password replacement")
require(linux_proxies, "移除认证", "Linux explicit authentication removal")
require(luci_proxies, "classifyLocalProxyListen", "LuCI local proxy address classification")
require(luci_proxies, "steer-exposure-warning", "LuCI local proxy exposure warning")
require(mac_editors, "classifyLocalProxyListen", "macOS local proxy address classification")
require(mac_editors, "validateLocalProxyAuthentication", "macOS local proxy authentication gate")
if "value: draft.password" in linux_proxies:
    raise SystemExit("check-ui-contract: Linux local proxy editor redisplays a saved password")

fixture_consumers = (
    "go/internal/intent/validate_test.go",
    "tests/node/linux_web_test.js",
    "tests/node/luci_view_test.js",
    "macos/SteerAppTests/LocalProxyAddressTests.swift",
)
for consumer in fixture_consumers:
    consumer_content = (ROOT / consumer).read_text()
    require(consumer_content, "local-proxy-listen-fixtures.json", consumer)

subscription_fixture_consumers = (
    "tests/node/linux_web_test.js",
    "tests/node/luci_view_test.js",
    "macos/SteerAppTests/SubscriptionStatusTests.swift",
)
for consumer in subscription_fixture_consumers:
    consumer_content = (ROOT / consumer).read_text()
    require(consumer_content, "subscription-status-fixtures.json", consumer)

probe_fixture_consumers = (
    "tests/node/linux_web_test.js",
    "tests/node/luci_view_test.js",
    "macos/SteerAppTests/ProbeDiagnosticsTests.swift",
)
for consumer in probe_fixture_consumers:
    consumer_content = (ROOT / consumer).read_text()
    require(consumer_content, "probe-diagnostics-fixtures.json", consumer)

state_fixture_consumers = (
    "tests/node/linux_web_test.js",
    "tests/node/luci_view_test.js",
    "macos/SteerAppTests/StateLifecycleTests.swift",
)
for consumer in state_fixture_consumers:
    consumer_content = (ROOT / consumer).read_text()
    require(consumer_content, "state-lifecycle-fixtures.json", consumer)

require(linux_ui, "Draft desired", "Linux Draft state")
require(linux_ui, "Saved desired", "Linux Saved state")
require(linux_ui, "重载最新 Saved", "Linux external revision reload")
require(luci_overview, "Draft / Saved / Active", "LuCI lifecycle model")
require(luci_overview, "Save & Apply pending changes", "LuCI pending Apply action")
require(luci_helper, "steer-lifecycle-global", "LuCI global lifecycle actions")
require(luci_helper, "applySaved", "LuCI Apply Saved action")
require(mac_content, "DraftActionButtons", "macOS lifecycle actions")

require(linux_diagnostics, "不证明具体 outbound", "Linux accurate probe boundary")
require(luci_overview, "does not prove a particular outbound", "LuCI accurate probe boundary")
require(mac_content, "不证明具体 outbound", "macOS accurate probe boundary")
require(linux_diagnostics, "diagnostics.reports", "Linux persisted probe reports")
require(luci_overview, "diagnostics.reports", "LuCI persisted probe reports")
require(mac_content, "diagnosticProbeReports", "macOS persisted probe reports")
require(luci_nodes, "probeOperationGate", "LuCI pending probe gate")
require(mac_content, "!item.enabled", "macOS disabled probe action")

require(linux_subscriptions, "last_failure", "Linux subscription failure state")
require(linux_subscriptions, "status.stale", "Linux subscription stale state")
require(luci_nodes, "subscriptionOperationGate", "LuCI pending subscription gate")
require(luci_nodes, "Running configuration was not changed", "LuCI no-Apply inventory warning")
require(mac_state, "SubscriptionStaleNode", "macOS stale node contract")
require(mac_content, "SubscriptionStaleList", "macOS per-node stale management")
require(mac_content, "pinned-stale", "macOS stale node badge")
for source, owner in (
    (linux_web_api, "Linux subscription API"),
    (openwrt_cli, "OpenWrt subscription CLI"),
    (mac_control, "macOS subscription control"),
):
    if 'json:"snapshots"' in source or 'json:"snapshot"' in source:
        raise SystemExit(f"check-ui-contract: {owner} exposes credential-bearing internal snapshots")
    require(source, '"subscriptions"', owner)

for content, owner in (
    (linux_nodes, "Linux nodes"),
    (luci_nodes, "LuCI nodes"),
    (mac_content, "macOS nodes"),
):
    require(content, "source_subscription" if owner != "macOS nodes" else "sourceSubscription", owner)

require(linux_nodes, "S.api.importNodes", "Linux multi-node import")
require(luci_nodes, "steer.importNodes", "LuCI multi-node import")
require(mac_state, "parseNodes(document:", "macOS multi-node import")
require(linux_nodes, "testButton('下载', true", "Linux node download test")
require(luci_nodes, "_download_speedtest", "LuCI node download test")
require(mac_content, "probeButton(item: item, scope: \"nodes\", download: true)", "macOS node download test")
require(linux_routes, "draft.kind = 'single'", "Linux fixed system routes")
require(luci_nodes, "addSystemRouteSection", "LuCI fixed system routes")
require(mac_content, "isSystemRoute(item)", "macOS fixed system routes")
require(mac_state, 'return "失败"', "macOS sanitized probe summary")
require(linux_lib, "详细原因请查看诊断日志", "Linux sanitized probe summary")
require(luci_nodes, "See diagnostic logs for details.", "LuCI sanitized probe summary")
if "report?.error || results.map" in linux_lib or "report?.error || results.map" in luci_nodes:
    raise SystemExit("check-ui-contract: a node list still exposes raw probe backend errors")

if (ROOT / "luci-app-steer/htdocs/luci-static/resources/steer/share-url.js").exists():
    raise SystemExit("check-ui-contract: LuCI retained its duplicate share URL parser")

print("shared native UI contract checks passed")
