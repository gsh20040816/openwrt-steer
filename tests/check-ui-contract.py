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
luci_navigation = []
for group_key, item_keys in expected_navigation:
    group_path = f"admin/services/steer/{group_key}"
    if group_path not in luci_menu:
        raise SystemExit(f"check-ui-contract: LuCI navigation misses {group_path}")
    children = sorted(
        (
            (entry["order"], path.rsplit("/", 1)[-1])
            for path, entry in luci_menu.items()
            if path.startswith(group_path + "/")
            and "/" not in path[len(group_path) + 1:]
        ),
        key=lambda item: item[0],
    )
    actual_keys = [key for _, key in children]
    if group_key == "advanced":
        # LuCI uses "configuration" to avoid a repeated /advanced/advanced URL.
        actual_keys = ["advanced" if key == "configuration" else key for key in actual_keys]
    if actual_keys != item_keys:
        raise SystemExit(
            f"check-ui-contract: LuCI {group_key} hierarchy drift: {actual_keys!r}"
        )

node_types = {item["value"] for item in contract["node_types"]}
if node_types != {
    "socks", "http", "shadowsocks", "vmess", "vless", "trojan", "hysteria",
    "hysteria2", "shadowtls", "tuic", "anytls", "naive", "ssh", "tor",
}:
    raise SystemExit("check-ui-contract: node protocol matrix is incomplete")

linux_ui = (ROOT / "go/cmd/steer-linux/web/js/ui.js").read_text()
linux_nodes = (ROOT / "go/cmd/steer-linux/web/js/views/nodes.js").read_text()
luci_nodes = (ROOT / "luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js").read_text()
mac_content = (ROOT / "macos/SteerApp/ContentView.swift").read_text()
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

if (ROOT / "luci-app-steer/htdocs/luci-static/resources/steer/share-url.js").exists():
    raise SystemExit("check-ui-contract: LuCI retained its duplicate share URL parser")

print("shared native UI contract checks passed")
