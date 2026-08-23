# SteerBox bridge

This module is the dependency boundary for the signed macOS NetworkExtension
runtime. It pins sing-box to `v1.13.19`; the root `go.mod` must remain free of
sing-box dependencies so OpenWrt and Linux builds do not inherit Libbox's
dependency graph.

The bridge will eventually build two products from this module:

- a Packet Tunnel runtime with a TUN inbound and no system DNS settings;
- a DNS-only runtime exposing the dedicated DNS inbound used by
  `NEDNSProxyProvider`.

The current C ABI exports `SteerValidateJSON`, `SteerCompileMacOS`,
`SteerPrepareMacOS`, `SteerABIVersion` and `SteerFreeString`. Every returned
string is allocated with `C.CString`; callers must release it exactly once with
`SteerFreeString`.

No source in this directory is claimed to build on the current Linux host.
The first macOS implementation must record the sing-box tag, Go version, build
tags, target architectures and SHA-256 of the resulting XCFramework.
