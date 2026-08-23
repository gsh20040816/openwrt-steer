# macOS targets

这里预留原生 macOS 工程目录。当前仓库没有 macOS 开发机，因此暂不提交未经 Xcode、签名和真实 NetworkExtension 授权验证的 `.xcodeproj` 或 provider 实现。

目标结构：

```text
Steer.app
├── SwiftUI/AppKit host
├── SteerNetwork.systemextension
│   ├── PacketTunnelProvider
│   └── DNSProxyProvider
└── SteerAgent
```

当前已提交的原生源码占位位于：

- `SteerApp/`：SwiftUI NavigationSplitView、菜单栏入口、draft/Validate/Apply 状态骨架；
- `SteerNetwork/`：Packet Tunnel 与 DNS Proxy provider 的职责边界、Info.plist 和 entitlements；
- `bridge/`：固定 sing-box `v1.13.19` 的独立 Go module 边界。

`SteerApp` 会在 bundle 中找到 `steer-macos` 时通过临时 canonical JSON 文件调用
`validate`；未嵌入 helper 时按钮保持明确的“未配置”错误，不会静默伪造校验通过。

实现约束：

- Packet Tunnel 不设置 `NEDNSSettings`；
- DNS Proxy 同时处理 UDP/TCP 53，并把已捕获流量转发到生成配置中的专用 DNS inbound；
- 两个 provider 共享 App Group 中的 generation/status/log 路径；
- Go 编译与校验通过 `go/cmd/steer-macos` 和 `go/internal/platform/macos` 完成；
- 首次提交必须在真实 Mac 上验证签名、App Group、安装、启动、停止、重启恢复和卸载。

当前没有 `.xcodeproj`，因为 Xcode 会根据实际 Developer Team、Bundle ID、App Group 和 System Extension target 生成签名元数据；这些值不能在没有 Apple 开发环境时凭空写入。现有 Swift 源码是工程输入，不是已构建或已签名的应用声明。
