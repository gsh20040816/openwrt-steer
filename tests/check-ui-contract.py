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

linux_ui = (ROOT / "go/cmd/steer-linux/web/js/ui.js").read_text()
linux_lib = (ROOT / "go/cmd/steer-linux/web/js/lib.js").read_text()
linux_nodes = (ROOT / "go/cmd/steer-linux/web/js/views/nodes.js").read_text()
linux_routes = (ROOT / "go/cmd/steer-linux/web/js/views/routes.js").read_text()
linux_subscriptions = (ROOT / "go/cmd/steer-linux/web/js/views/subscriptions.js").read_text()
luci_nodes = (ROOT / "luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js").read_text()
mac_content = (ROOT / "macos/SteerApp/ContentView.swift").read_text()
mac_state = (ROOT / "macos/SteerApp/AppState.swift").read_text()
mac_editors = (ROOT / "macos/SteerApp/DraftEditors.swift").read_text()

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
