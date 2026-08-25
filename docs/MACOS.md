# macOS 开发基线

macOS 由两个正式组成部分构成：SwiftUI GUI 是用户前端，root LaunchDaemon + 外部 sing-box 是运行后端。GUI 与 OpenWrt LuCI、Linux Web 处于同一层，只负责配置、操作和状态展示，不承载代理数据面。

```text
Steer GUI
  ├── 编辑 Canonical Intent
  ├── Validate / Save / Apply
  └── Status / Diagnostics
             ↓ macOS 管理员授权
/usr/local/libexec/steer/steer-macos
             ↓ launchctl bootstrap
root LaunchDaemon: steer-macos _run
             ↓ exec
sing-box run -c current/sing-box.json
             ↓
Darwin utun + auto_route
```

这条路径不要求 Apple Developer Program 或付费签名。sing-box 的 TUN inbound 官方支持 macOS；`auto_redirect` 是 Linux 路径，macOS 不使用它。[sing-box Tun](https://sing-box.sagernet.org/configuration/inbound/tun/)

## GUI 前端

`macos/SteerApp` 是 macOS 的正式配置与运维前端，不是代理运行时，也不维护第二份配置语义。它直接面向系统安装的 `steer-macos` helper：

- Overview：启用状态、当前 generation、健康状态和 Apply；
- Configuration：读取、保存和编辑 Canonical JSON；
- Nodes、Routes、DNS、Rules、Subscriptions、Local Proxies：编辑同一份 draft collection；
- Diagnostics：显示共享校验结果和 LaunchDaemon 后端状态；
- Settings：显示系统配置、Geo 目录和授权边界。

读取系统配置、Save、Apply 和 Status 使用 macOS 标准管理员授权。Validate 可直接调用 helper 校验临时 draft。GUI 不直接写运行 generation，不直接启动 sing-box，也不复制 Go 校验或编译逻辑。

系统配置的唯一真相仍是：

```text
/Library/Application Support/Steer/config/config.json
```

## DNS 路径

macOS 不复制 Linux 的 nftables `PREROUTING`/`OUTPUT` shim，也不引入 SmartDNS。DNS 由同一份 sing-box DNS Router 处理，Steer 在 TUN inbound 上只对明确的 TCP/UDP 目标端口 53 生成 `hijack-dns` 规则：

```text
应用 / 系统 resolver
        ↓
macOS utun
        ↓
sing-box route: inbound=steer-tun, tcp/udp, port=53
        ↓
sing-box DNS Router
        ↓
DNS Profile → Route / outbound
```

这不会把普通 UDP session 当成 DNS。DNS Profile、缓存、detour 和上游协议继续由 sing-box 内部实现。

## Go macOS adapter

`go/internal/platform/macos` 负责：

- Darwin TUN plan：地址、MTU、`auto_route` 和非全球地址排除；`198.18.0.0/15` 包含 system stack 自身的 IPv4 对端，不能加入排除表；
- 显式 TCP/UDP 53 DNS capture；
- sing-box version/capability/check；
- generation prepare/publish；
- launchd stop/bootstrap；
- utun 地址和 LaunchDaemon health；
- atomic Apply record、status 和 cleanup。

默认运行目录为：

```text
/Library/Application Support/Steer/config/config.json
/Library/Application Support/Steer/run/
/Library/Application Support/Steer/state/
/Library/Application Support/Steer/geodata-seed/manifest.json
/Library/Application Support/Steer/geodata-seed/rules/*.srs
```

`_run` 只供 LaunchDaemon 使用。它在冷启动时准备 current generation，然后直接 `exec` sing-box，不成为第二个 supervisor。

## 安装和开发

先安装 sing-box，再安装 helper 和 LaunchDaemon：

```sh
brew install sing-box
sudo macos/scripts/install-launchdaemon.sh
```

安装器把 `command -v sing-box` 选中的构件复制到 root-owned `/usr/local/libexec/steer/sing-box`；GUI、CLI 和 LaunchDaemon 都使用这份运行时，更新 sing-box 后需重新运行安装器。

构建 GUI：

```sh
cd macos
swift build --disable-sandbox
swift run SteerApp
```

也可以直接使用 helper：

```sh
sudo /usr/local/libexec/steer/steer-macos validate
sudo /usr/local/libexec/steer/steer-macos apply
sudo /usr/local/libexec/steer/steer-macos health
sudo /usr/local/libexec/steer/steer-macos status
```

## 限制

当前 macOS 目标明确不支持：

- `source_mac_address`；
- Linux `auto_redirect`、nftables、pf；
- SmartDNS 独立进程；
- 应用自带 DoH/DoQ 的明文查询识别。

Geo 表达式可以使用。macOS 需要把与当前 master 构建匹配的 `geodata-seed/` 放入上述目录；Geo 转换在 CI/release 阶段完成，目标机不安装 geoview，也不读取 DAT。
