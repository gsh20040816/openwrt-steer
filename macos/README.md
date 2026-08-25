# macOS targets

macOS 的正式产品由 SwiftUI GUI 前端和 LaunchDaemon 后端组成：

```text
SteerApp
  └── config / validate / apply / status
             ↓ administrator authorization
steer-macos helper
  └── launchd lifecycle → external sing-box → Darwin TUN
```

GUI 与 OpenWrt LuCI、Linux Web 同级：它编辑同一份 Canonical Intent，并调用平台后端完成校验、保存、Apply 和状态读取。GUI 不包含代理数据面，也不复制 Go 核心语义。

当前源码入口：

- `SteerApp/`：SwiftUI 配置与运维前端；
- `../go/internal/platform/macos/`：Darwin TUN、DNS port-53 capture、generation 和 launchd backend；
- `launchd/`：root LaunchDaemon plist；
- `scripts/install-launchdaemon.sh`：构建 helper、发现 sing-box、安装并 bootstrap 服务；
- `../go/cmd/steer-macos/`：macOS 后端 CLI。

## 安装和运行

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

GUI 通过 macOS 管理员授权访问 `/Library/Application Support/Steer/config/config.json` 和已安装的 helper。安装脚本会根据 `command -v sing-box` 自动处理 Apple Silicon 与 Intel Homebrew 前缀，并把选中的构件复制为 root-owned `/usr/local/libexec/steer/sing-box`，确保 GUI、CLI 与 LaunchDaemon 使用同一份不可由普通用户替换的运行时。

## TUN、DNS 和 Geo

sing-box 负责 Darwin utun 和 `auto_route`；macOS plan 不设置 `auto_redirect`、nftables 或 pf。TUN 使用 `198.18.0.1/30` 时，排除表不得覆盖其 system-stack 对端 `198.18.0.2`。DNS 由 sing-box 内部 DNS Router 处理，Steer 只生成明确匹配 TUN 上 TCP/UDP 53 的 `hijack-dns` route rule。

Geo 不是 macOS 语义限制。若 Intent 使用 `geosite:`/`geoip:`，把与当前 master 构建匹配的 `geodata-seed/` 放入 `/Library/Application Support/Steer/geodata-seed/`。Apply 会按 manifest 校验所需 SRS；目标机不安装 geoview，也不读取 DAT。
