#!/usr/bin/env python3
"""Static checks for macOS source contracts that do not require a Mac."""

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def read(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def main() -> None:
    plan = read("go/internal/platform/macos/plan.go")
    compiler = read("go/internal/compiler/compiler.go")
    packet = read("macos/SteerNetwork/PacketTunnelProvider.swift")
    dns = read("macos/SteerNetwork/DNSProxyProvider.swift")
    bridge = read("macos/bridge/go.mod")
    packet_info = read("macos/SteerNetwork/PacketTunnel-Info.plist")
    dns_info = read("macos/SteerNetwork/DNSProxy-Info.plist")

    assert 'Mode:        compiler.DNSCaptureInboundHijack' in plan
    assert '"auto_route"' not in plan
    assert '"auto_redirect"' not in plan
    assert 'DNSCaptureInboundHijack' in compiler
    assert 'settings.dnsSettings = nil' in packet
    assert 'handleNewFlow(_ flow: NEAppProxyFlow)' in dns
    assert 'github.com/sagernet/sing-box v1.13.19' in bridge
    assert 'com.apple.networkextension.packet-tunnel' in packet_info
    assert 'com.apple.networkextension.dns-proxy' in dns_info
    print("macOS source contracts passed")


if __name__ == "__main__":
    main()
