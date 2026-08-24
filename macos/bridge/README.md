# SteerBox bridge

This module is an optional future native-runtime boundary. If it later embeds
sing-box, it must use the supported `v1.14.0-rc.1` baseline; the root `go.mod` must remain free of sing-box dependencies so
OpenWrt, Linux, and the supported macOS LaunchDaemon path do not inherit
Libbox's dependency graph.

If Apple Developer entitlements become available in a future version, the
bridge may build two products from this module:

- a Packet Tunnel runtime with a TUN inbound and no system DNS settings;
- a DNS-only runtime exposing the dedicated DNS inbound used by
  `NEDNSProxyProvider`.

The current C ABI exports `SteerValidateJSON`, `SteerCompileMacOS`,
`SteerPrepareMacOS`, `SteerABIVersion` and `SteerFreeString`. Every returned
string is allocated with `C.CString`; callers must release it exactly once with
`SteerFreeString`.

The bridge is intended to build on the macOS host through the repository's
XCFramework script. The first signed implementation must record the sing-box
tag, actual Go version, build tags, target architectures and SHA-256 of the
resulting XCFramework.
