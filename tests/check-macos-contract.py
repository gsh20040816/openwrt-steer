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
    launchd = read("macos/launchd/com.steer.steer.plist")
    installer = read("macos/scripts/install-launchdaemon.sh")
    package = read("macos/Package.swift")
    agent = read("macos/SteerAgent/AgentController.swift")
    agent_plist = read("macos/SteerAgent/com.steer.steer.agent.plist")
    app = read("macos/SteerApp/SteerApp.swift")
    app_info = read("macos/SteerApp/Info.plist")
    state = read("macos/SteerApp/AppState.swift")
    content = read("macos/SteerApp/ContentView.swift")

    assert 'DNSCaptureInboundHijack' in compiler
    assert 'DNSCaptureTUNPort53Hijack' in macos_plan
    assert '"auto_route": true' in macos_plan
    assert '"auto_redirect"' not in macos_plan
    assert 'launchctl' in macos_backend
    assert 'geodata.ValidateRequiredRules' in macos_backend
    assert 'PLATFORM_UNSUPPORTED_GEO_TOOLCHAIN' not in macos_validate
    assert 'case "apply"' in macos_cli
    assert 'case "_run"' in macos_cli
    assert 'com.steer.steer' in launchd
    assert 'com.steer.steer.agent' in agent_plist
    assert '<string>com.steer.steer</string>' in app_info
    legacy_identifier = "com." + "gsh20040816.steer"
    assert legacy_identifier not in launchd
    assert legacy_identifier not in agent
    assert 'command -v sing-box' in installer
    assert 'runtime_binary="$helper_directory/sing-box"' in installer
    assert '/usr/local/libexec/steer/sing-box' in launchd
    assert 'plutil -replace ProgramArguments' not in installer
    assert 'SMAppService.agent' in agent
    assert '.executable(name: "SteerApp"' in package
    assert "SteerNetwork" not in package
    assert 'NavigationSplitView' in content
    assert 'HelperBackendClient' in state
    assert 'with administrator privileges' in state
    assert '"apply"' in state and '"status"' in state
    assert '/Library/Application Support/Steer/config/config.json' in state
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
