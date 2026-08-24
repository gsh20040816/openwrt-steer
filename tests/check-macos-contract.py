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
    launchd = read("macos/launchd/com.gsh20040816.steer.plist")
    installer = read("macos/scripts/install-launchdaemon.sh")
    bridge = read("macos/bridge/go.mod")
    bridge_source = read("macos/bridge/bridge.go")
    root_mod = read("go/go.mod")
    agent = read("macos/SteerAgent/AgentController.swift")
    build_script = read("macos/scripts/build-steercore-xcframework.sh")
    app = read("macos/SteerApp/SteerApp.swift")
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
    assert 'com.gsh20040816.steer' in launchd
    assert 'command -v sing-box' in installer
    assert 'v1.13.19' not in bridge
    assert 'v1.14.0-rc.1' in build_script
    assert 'github.com/sagernet/sing-box' not in root_mod
    assert 'SMAppService.agent' in agent
    for symbol in ("SteerValidateJSON", "SteerCompileMacOS", "SteerPrepareMacOS", "SteerFreeString"):
        assert symbol in bridge_source
    assert "GOARCH=\"$arch\"" in build_script
    assert "SteerCore.xcframework" in build_script
    assert 'NavigationSplitView' in content
    for page in ("Overview", "Configuration", "Nodes", "Routes", "DNS", "Rules", "Subscriptions", "Diagnostics", "Settings"):
        assert page in app or page in state or page in content
    assert "Local Proxies" in state
    print("macOS source contracts passed")


if __name__ == "__main__":
    main()
