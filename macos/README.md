# macOS targets

macOS 的正式产品由 SwiftUI GUI 前端和 LaunchDaemon 后端组成：

```text
SteerApp
  ├── read config / validate / status（无授权弹窗）
  ├── 首次安装 embedded payload（一次管理员授权）
  └── save / apply / subscription update-clean（后续免密）
             ↓ /var/run/steer/control.sock
steer-macos _control（root、仅允许固定配置与订阅操作）
             ↓
steer-macos helper
  └── launchd lifecycle → external sing-box → Darwin TUN
```

GUI 与 OpenWrt LuCI、Linux Web 同级：它编辑同一份 Canonical Intent，并调用平台后端完成校验、保存、Apply 和状态读取。GUI 不包含代理数据面，也不复制 Go 核心语义。

日常配置使用原生字段编辑器：基础设置覆盖 Main、探测、DNS 缓存与 Bootstrap；节点、路由、DNS Profile、本地代理、规则和订阅分别使用原生 Form。内部 Canonical ID 由 GUI 管理，不出现在普通列表或弹窗；完整 JSON 仅保留为侧栏“高级”区域的兜底入口。全局 Enable 开关变化会立即保存并 Apply，失败时恢复原状态。

当前源码入口：

- `SteerApp/`：SwiftUI 配置与运维前端；
- `../go/internal/platform/macos/`：Darwin TUN、DNS port-53 capture、generation 和 launchd backend；
- `launchd/`：运行、常驻 root control 与订阅调度 LaunchDaemon plist；
- `scripts/build-app-bundle.sh`：唯一的 App/DMG 组装、验收和 ad-hoc 签名脚本；
- `scripts/install-embedded-payload.sh`：正式 App 内置的固定 payload 安装器；
- `scripts/install-launchdaemon.sh`：源码开发时构建 helper、发现 sing-box、安装并 bootstrap 服务；
- `../go/cmd/steer-macos/`：macOS 后端 CLI。

## 正式 DMG

tag workflow 在原生 Apple Silicon 和 Intel runner 上分别生成：

```text
steer-macos-arm64.dmg
steer-macos-x86_64.dmg
```

将 App 拖入 `/Applications` 后，首次在“系统”页安装系统组件并输入一次管理员密码。之后 GUI Save/Apply 与订阅更新/清理经 `root:admin 0660` socket 和 Darwin peer credentials 保护的结构化 IPC 完成，不执行任意 shell 命令，也不再请求密码。

当前只有 ad-hoc 签名，没有 Developer ID 和 notarization。用户仍需按 macOS 未认证开发者流程首次确认；GitHub artifact attestation 可验证来源，但不会让 Gatekeeper 自动放行。

## 源码安装和运行

先安装外部 sing-box：

```sh
brew install sing-box
```

再安装后端：

```sh
sudo macos/scripts/install-launchdaemon.sh
sudo /usr/local/libexec/steer/steer-macos validate
sudo /usr/local/libexec/steer/steer-macos apply
sudo /usr/local/libexec/steer/steer-macos health
```

构建并启动 GUI：

```sh
cd macos
swift build --disable-sandbox
swift run SteerApp
```

安装器把配置设为 `root:admin 0640`，并公开不含密钥的 generation 摘要，因此 GUI 启动、刷新状态、Validate、探测和 Geo catalog 不需要管理员授权。开发安装脚本会根据 `command -v sing-box` 自动处理 Apple Silicon 与 Intel Homebrew 前缀，把选中的构件复制为 root-owned `/usr/local/libexec/steer/sing-box`，并安装 control 与订阅调度服务；后续 Save/Apply 与订阅更新/清理和正式 App 使用同一受限免密 IPC。

## TUN、DNS 和 Geo

sing-box 负责 Darwin utun 和 `auto_route`；macOS plan 不设置 `auto_redirect`、nftables 或 pf。TUN 使用 `198.18.0.1/30` 时，排除表不得覆盖其 system-stack 对端 `198.18.0.2`，但必须继续排除 LAN 网段，使非 DNS 局域网流量完全不进入代理核心。TUN 显式使用 sing-box 1.14 `dns_mode=hijack` 安装 Apple interface DNS 路径，Steer 仍生成只匹配 TUN 上 TCP/UDP **目标端口** 53 的 `hijack-dns` route rule；不使用源端口或 DNS 协议嗅探。

Geo 不是 macOS 语义限制。正式 DMG 已内置并校验 tag workflow 使用的精确 `geodata-seed/`；源码开发需把相同 seed 放入 `/Library/Application Support/Steer/geodata-seed/`。Apply 会按 manifest 校验所需 SRS；目标机不安装 geoview，也不读取 DAT。
