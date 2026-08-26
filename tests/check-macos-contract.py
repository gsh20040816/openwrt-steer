#!/usr/bin/env python3
"""Static checks for macOS source contracts that do not require a Mac."""

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def read(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def main() -> None:
    compiler = read("go/internal/compiler/compiler.go")
    macos_plan = read("go/internal/platform/macos/plan.go")
    macos_backend = read("go/internal/platform/macos/backend.go")
    macos_validate = read("go/internal/platform/macos/validate.go")
    macos_cli = read("go/cmd/steer-macos/main.go")
    control = read("go/cmd/steer-macos/control.go")
    control_peer = read("go/cmd/steer-macos/control_peer_darwin.go")
    launchd = read("macos/launchd/com.steer.steer.plist")
    control_launchd = read("macos/launchd/com.steer.steer.control.plist")
    subscription_launchd = read("macos/launchd/com.steer.steer.subscription.plist")
    installer = read("macos/scripts/install-launchdaemon.sh")
    embedded_installer = read("macos/scripts/install-embedded-payload.sh")
    package = read("macos/Package.swift")
    ci = read(".github/workflows/ci.yml")
    app = read("macos/SteerApp/SteerApp.swift")
    app_info = read("macos/SteerApp/Info.plist")
    state = read("macos/SteerApp/AppState.swift")
    content = read("macos/SteerApp/ContentView.swift")
    editors = read("macos/SteerApp/DraftEditors.swift")
    ui_spec = read("macos/SteerApp/UISpec.swift")
    general = read("macos/SteerApp/ConfigurationFormView.swift")

    assert 'DNSCaptureInboundHijack' in compiler
    assert 'DNSCaptureTUNPort53Hijack' in macos_plan
    assert '"dns_mode": "disabled"' in macos_plan
    assert 'DirectRouteAddress' in macos_plan
    for private_prefix in ("10.0.0.0/8", "100.64.0.0/10", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"):
        assert private_prefix in macos_plan
    assert '"fe80::/10"' in macos_plan
    for retired_lan_state in ("DiscoverActiveLANPrefixes", "ActiveLANPrefixes", "LANPrefixes", "monitorLANPrefixes", "reconcileLANPrefixes"):
        assert retired_lan_state not in macos_plan
        assert retired_lan_state not in macos_backend
        assert retired_lan_state not in control
    assert not (ROOT / "go/internal/platform/macos/lan.go").exists()
    assert '"auto_route": true' in macos_plan
    assert '"auto_redirect"' not in macos_plan
    assert 'launchctl' in macos_backend
    assert 'geodata.ValidateRequiredRules' in macos_backend
    assert 'PLATFORM_UNSUPPORTED_GEO_TOOLCHAIN' not in macos_validate
    assert 'case "apply"' in macos_cli
    assert 'case "parse-nodes"' in macos_cli
    assert 'case "probe"' in macos_cli
    assert 'case "geo-catalog"' in macos_cli
    assert 'case "subscription"' in macos_cli
    assert 'case "verify-geodata"' in macos_cli
    assert 'case "control"' in macos_cli
    assert 'case "_control"' in macos_cli
    assert 'case "_run"' in macos_cli
    assert 'com.steer.steer' in launchd
    assert 'com.steer.steer.control' in control_launchd
    assert 'com.steer.steer.subscription' in subscription_launchd
    assert '<key>StartInterval</key>' in subscription_launchd
    assert '<string>_control</string>' in control_launchd
    assert '<string>/var/run/steer/control.sock</string>' in control_launchd
    assert '<key>KeepAlive</key>' in control_launchd and '<true/>' in control_launchd
    assert '<string>com.steer.steer</string>' in app_info
    legacy_identifier = "com." + "gsh20040816.steer"
    assert legacy_identifier not in launchd
    assert legacy_identifier not in control_launchd
    assert 'command -v sing-box' in installer
    assert 'runtime_binary="$helper_directory/sing-box"' in installer
    assert '/usr/local/libexec/steer/sing-box' in launchd
    assert 'plutil -replace ProgramArguments' not in installer
    assert 'while launchctl print system/com.steer.steer' in installer
    assert 'Timed out waiting for the previous Steer LaunchDaemons to stop.' in installer
    assert '.executable(name: "SteerApp"' in package
    assert 'SteerAgent' not in package
    assert "SteerNetwork" not in package
    assert 'NavigationSplitView' in content
    assert 'navigationSplitViewColumnWidth(min: 200' in content
    assert 'Table(of: DraftItem.self, selection: $selection)' in content
    assert 'descriptor.ordered && !isPinned(item) ? NSItemProvider(object: item.id as NSString) : nil' in content
    assert '.dropDestination(for: String.self)' in content
    assert '"line.3.horizontal"' in content and '"pin.fill"' in content
    assert 'List {' in content and 'Section {' in content
    assert 'GroupBox' not in content
    for editor in ("NodeDraftForm", "RouteDraftForm", "DNSDraftForm", "LocalProxyDraftForm", "RuleDraftForm", "SubscriptionDraftForm"):
        assert editor in editors
    for editor_control in ("TextField", "SecureField", "Picker", "Toggle", "DisclosureGroup"):
        assert editor_control in editors
    assert 'Canonical JSON 条目' not in editors
    assert 'TextField("ID"' not in editors
    assert 'Text(item.identifier)' not in content
    assert 'ConfigurationFormView' in content
    assert 'Bootstrap DNS' in general and 'DNS 缓存' in general and '连通性探测' in general
    assert 'setEnabledAndApply($0)' not in general and 'setEnabledAndApply($0)' in content
    assert 'func setEnabledAndApply' in state
    assert 'deletionBlockReason' in state and '.alert(' in content
    assert 'NodeImportSheet' in editors and 'parseNodes(document:' in state
    assert 'sourceSubscription' in state and 'NodeCollectionGroup' in content
    assert 'runAllNodeProbes(download: Bool, nodeIDs: [String])' in state
    assert 'probeInProgress(scope:' in state and 'download: true' in content
    assert 'activeProbeKeys' in state and 'perform(message: "正在运行探测…")' not in state
    assert 'guard let result = results.first(where: { $0.ok }) else {' in state and 'return "失败"' in state
    assert 'isSystemRoute(item)' in content and 'isDefaultRule(item)' in content
    assert '系统必需 · 始终启用' in editors
    assert 'Picker("类型"' not in editors and 'LabeledContent("类型", value: "Single 节点")' in editors
    assert 'SharedNodeDraftForm' in editors and 'SteerUISpec.nodeFields' in editors
    dns_editor = editors[editors.index("private struct DNSDraftForm"):editors.index("private struct LocalProxyDraftForm")]
    assert 'UIDNSProtocolSpec' in ui_spec
    assert 'applyDNSProtocol' in ui_spec and 'normalizeDNSProfile' in ui_spec
    assert 'defaultPort' in ui_spec and 'protocolSpec?.fields.contains' in dns_editor
    assert 'currentPort == $0.defaultPort' in ui_spec and 'protocolSpec?.defaultPort' in dns_editor
    assert '["tls", "https", "quic", "h3"]' not in dns_editor
    assert 'func probe(kind:' in state
    assert 'func subscriptionStatuses()' in state
    assert 'func updateSubscription(id:' in state
    assert 'func geoCatalog(kind:' in state
    assert 'HelperBackendClient' in state
    assert 'with administrator privileges' in state
    assert '"--expected-revision", expectedRevision' in state
    assert 'ConfigurationSnapshot' in state and 'savedRevision' in state
    assert 'case revisionConflict(currentRevision: String)' in state
    for conflict_choice in ("Reload Saved", "保留本地 Draft", "显式覆盖"):
        assert conflict_choice in content
    assert 'executePrivileged(Self.command([installer.path]))' in state
    assert 'executePrivileged("\\(install)' not in state
    assert 'Data(contentsOf: url)' in state
    assert 'Self.execute(helper, ["status"])' in state
    assert '"apply"' in state and '"status"' in state
    assert '/Library/Application Support/Steer/config/config.json' in state
    assert 'openWindow(id: "main")' in app
    assert '.testTarget(' in package
    assert (ROOT / "macos/SteerAppTests/AppStateRevisionTests.swift").exists()
    assert 'swift test --disable-sandbox' in ci
    assert 'install -d -o root -g admin -m 0750' in installer
    assert 'install -d -o root -g wheel -m 0755 "/var/run/steer"' in installer
    assert 'com.steer.steer.control.plist' in installer
    assert 'chmod 0640 "$support_directory/config/config.json"' in installer
    assert 'chmod 0644 "$support_directory/run/current.json"' in installer
    assert 'atomicWriteMode(filepath.Join(paths.Root, "current.json"), encoded, 0o644)' in read("go/internal/platform/macos/generation.go")
    assert 'request.Operation != "save" && request.Operation != "apply"' in control
    assert 'request.Operation != "subscription-update"' in control
    assert 'request.Operation != "subscription-clean"' in control
    assert 'ExpectedRevision' in control and 'currentControlRevision' in control
    assert 'REVISION_CONFLICT' in control and 'EXPECTED_REVISION_REQUIRED' in control
    assert 'decoder.DisallowUnknownFields()' in control
    assert 'maxControlDocument' in control and 'maxControlMessage' in control
    assert 'authorizedControlPeer' in control
    assert 'os.Geteuid() != 0' in control
    assert 'os.Chmod(*socketPath, 0o660)' in control
    assert 'os.Chown(*socketPath, 0, adminGID)' in control
    assert 'unix.GetsockoptXucred' in control_peer
    assert 'unix.LOCAL_PEERCRED' in control_peer
    assert 'command -v' not in embedded_installer
    assert 'go build' not in embedded_installer
    assert 'PAYLOAD-SHA256SUMS' in embedded_installer
    assert 'verify-geodata --directory' in embedded_installer
    assert 'if [ -f "$support_directory/config/config.json" ]' in embedded_installer
    assert 'launchctl bootstrap system "$control_plist_path"' in embedded_installer
    assert '安装系统组件' in content
    assert '受限 Unix socket IPC' in content
    for page in ("Overview", "Configuration", "Nodes", "Routes", "DNS", "Rules", "Subscriptions", "Diagnostics", "Settings"):
        assert page in app or page in state or page in content
    assert "Local Proxies" in state

    removed_paths = (
        "macos/SteerNetwork",
        "macos/SteerApp/NetworkExtensionController.swift",
        "macos/SteerApp/SteerApp.entitlements",
        "macos/bridge",
        "macos/scripts/build-steercore-xcframework.sh",
        "go/pkg/steermacos",
        "go/internal/platform/macos/bridge.go",
        "go/internal/platform/macos/dns.go",
        "macos/SteerAgent",
    )
    for relative in removed_paths:
        assert not (ROOT / relative).exists(), f"obsolete macOS runtime path remains: {relative}"

    forbidden = ("NetworkExtension", "PacketTunnel", "DNSProxy", "network_extension", "dns_proxy", "App Group")
    checked_paths = (
        ROOT / "macos",
        ROOT / "go/internal/platform/macos",
        ROOT / "go/cmd/steer-macos",
    )
    for base in checked_paths:
        for path in base.rglob("*"):
            if not path.is_file() or ".build" in path.parts:
                continue
            content = path.read_text(encoding="utf-8")
            for token in forbidden:
                assert token not in content, f"obsolete token {token!r} remains in {path.relative_to(ROOT)}"
    print("macOS source contracts passed")


if __name__ == "__main__":
    main()
