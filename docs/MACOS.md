# macOS 开发基线

本文记录 macOS 开发的当前边界。macOS 目标使用原生 SwiftUI/AppKit 界面和 Apple NetworkExtension；共享 Canonical Intent、校验和 sing-box 配置编译仍由 Go 核心负责。

DNS 方案的正式决策记录在 [ADR 0001](adr/0001-macos-dns-capture.md)。

## 运行时边界

```text
Steer.app
├── 原生 GUI：编辑 draft、Validate、Apply、状态与诊断
├── Agent：订阅、探测、Geo/Tor 等后台任务
└── Network Extension
    ├── PacketTunnelProvider：接管普通应用流量
    └── DNSProxyProvider：先捕获传统 UDP/TCP DNS
```

Packet Tunnel 由 NetworkExtension 创建和配置 utun。macOS 目标不使用 Linux 的 `auto_route`、`auto_redirect`、nftables、pf 或 root daemon，也不把 DNS 设置安装到 Packet Tunnel 的网络设置中。系统路由和 DNS Proxy 授权必须在真实签名的 Mac 上验证；当前开发环境没有 macOS，因此这里只提交可在 Linux 上验证的配置契约。

## DNS 路径

DNS 采用两层边界，但只保留一份 sing-box DNS Router：

```text
系统 resolver / 应用
        ↓
NEDNSProxyProvider（第一层：确认 UDP/TCP DNS）
        ↓
Steer 专用 loopback DNS inbound
        ↓
sing-box route action: hijack-dns
        ↓
sing-box DNS Router（DNS Profile、Route、缓存和上游协议）
```

`hijack-dns` 只匹配专用 DNS inbound，不匹配 Packet Tunnel 中的普通 UDP session。这样与 Linux/OpenWrt 的“先按目标端口分流，再交给专用 inbound”语义一致，同时避免把 TUN 会话识别为 DNS。

共享 compiler 的 `DNSCapture` 只描述 sing-box 需要的 inbound tags 和模式，不包含 nftables、NetworkExtension 或 Swift 类型。平台适配器负责第一层捕获：

- Linux/OpenWrt：nftables 将 UDP/TCP 53 转到专用 inbound；
- macOS：`NEDNSProxyProvider` 将捕获的 UDP/TCP flow 转到 `steer-dns4`/`steer-dns6`；
- 其他测试 runtime：使用 `DNSCaptureNone`，不生成 `hijack-dns` 规则。

共享 compiler 不再在缺少 Geo 状态目录时猜测 `/var/lib/steer`。含 Geo 规则的调用必须显式传入平台状态目录；macOS 调用方应传入 App Group 的 state container。

macOS 的 DNS Proxy 必须同时覆盖 UDP 和 TCP，并在 Swift 层处理 TCP 的两字节长度 framing、半包、粘包、超时和取消。DNS message 解析、规则选择、缓存和上游传输不在 Swift 中重写。

## 当前代码骨架

`go/internal/platform/macos` 目前包含纯 Go 的 `Plan`、JSON bridge、App Group store、generation prepare/publish 和 DNS framing：

- TUN inbound 不声明 `auto_route`/`auto_redirect`，由 NetworkExtension 负责系统路由；
- DNS inbound 绑定 `127.0.0.1:1053` 与 `[::1]:1054`；
- target 明确使用 `DNSCaptureInboundHijack`；
- `go/pkg/steercore` 提供版本化 `{abi_version, ok, value/error}` JSON envelope；
- `go test` 可在 Linux 上验证配置中存在专用 DNS `hijack-dns`，且不存在 `auto_redirect`；
- `macos/SteerApp` 提供 SwiftUI 页面、菜单栏入口、draft/Validate/Apply 状态和 provider manager 控制骨架；
- `macos/SteerNetwork` 提供 Packet Tunnel/DNS Proxy target 输入、entitlements 和 Info.plist 模板；
- `macos/bridge/go.mod` 将 sing-box 固定在 `v1.13.19`，不污染根 Go module。

这不是 macOS 系统扩展的完成声明。没有真实 Mac、Apple Developer 签名、App Group 和 NetworkExtension 授权时，不能声称 Packet Tunnel/DNS Proxy 已启动或端到端可用。

## 尚未完成但已留好接口的部分

- DNSProxyProvider 的 `NEAppProxyUDPFlow`/`NEAppProxyTCPFlow` 实际读写循环；
- Packet Tunnel 中 Libbox command server、utun fd handoff 和物理接口绑定；
- Swift 逐字段 schema 7 编辑器、订阅刷新/Geo/Probe/Agent；当前原生页面已支持 draft collection 的新增、删除、排序和 canonical JSON 保存。
- XCFramework 产物、Developer ID 签名、notarization 和真实 provider 健康检查。

这些部分需要 macOS SDK、Apple entitlement、签名身份或真实流量，当前环境无法诚实完成验证，因此代码中保持 fail-fast 占位，不把失败伪装成可用。

## 后续顺序

1. 在真实 Mac 上验证空 PacketTunnelProvider 与 DNSProxyProvider 的签名、App Group、安装、启动、停止和重启恢复；
2. 为两个 provider 接入同一 generation ID，并实现 DNS Proxy 到专用 inbound 的 UDP/TCP 转发；
3. 固定 sing-box/Libbox 版本，建立 Swift/Go JSON round-trip 和配置 golden test；
4. 再实现原生 GUI、Agent、订阅、探测和 Geo 能力。
