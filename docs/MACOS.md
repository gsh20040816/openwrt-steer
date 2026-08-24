# macOS 开发基线

macOS 的正式实现不依赖 Apple Developer Program、NetworkExtension entitlement 或付费签名。运行时采用普通 sing-box 二进制、Darwin TUN/`auto_route` 和 root LaunchDaemon：

```text
steer-macos apply
        ↓
launchctl bootstrap
        ↓
LaunchDaemon: steer-macos _run
        ↓
exec sing-box run -c current/sing-box.json
        ↓
macOS utun + auto_route
```

sing-box 的 TUN inbound 官方支持 macOS；`auto_redirect` 是 Linux 路径，macOS 不使用它。[sing-box Tun](https://sing-box.sagernet.org/configuration/inbound/tun/)

## DNS 路径

macOS 不复制 Linux 的 nftables `PREROUTING`/`OUTPUT` shim，也不引入 SmartDNS。DNS 仍由同一份 sing-box DNS Router 处理，Steer 在 TUN inbound 上只对明确的 TCP/UDP 目标端口 53 生成 `hijack-dns` 规则：

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

这不会把普通 UDP session 当成 DNS，也不需要 Swift 重写 DNS 报文。DNS Profile、缓存、detour 和上游协议继续由 sing-box 内部实现。

## Go macOS adapter

`go/internal/platform/macos` 负责：

- Darwin TUN plan：地址、MTU、`auto_route` 和非全球地址排除；
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

公共命令：

```text
version validate compile prepare apply health status cleanup
```

`_run` 只供 LaunchDaemon 使用。它在冷启动时准备 current generation，然后直接 `exec` sing-box，不成为第二个 supervisor。

## 安装

先安装 sing-box。官方提供 Homebrew 安装方式：[sing-box package manager](https://sing-box.sagernet.org/installation/package-manager/)

然后以 root 执行：

```sh
sudo macos/scripts/install-launchdaemon.sh
```

安装脚本会：

1. 构建 `/usr/local/libexec/steer/steer-macos`；
2. 复制 LaunchDaemon plist；
3. 自动发现 `sing-box` 路径，兼容 Apple Silicon 和 Intel Homebrew；
4. 创建 Steer 配置、runtime、state、Geo seed 和日志目录；
5. 执行 `launchctl bootstrap`。

配置文件应由用户或上层 UI 写入：

```text
/Library/Application Support/Steer/config/config.json
```

应用配置：

```sh
sudo /usr/local/libexec/steer/steer-macos apply
sudo /usr/local/libexec/steer/steer-macos health
sudo /usr/local/libexec/steer/steer-macos status
```

## 限制

当前 macOS 目标明确不支持：

- `source_mac_address`；
- Geo 表达式可以使用；macOS 需要把与当前 master 构建匹配的 `geodata-seed/`（含 `manifest.json` 和 `rules/*.srs`）放入上述目录。Geo 转换在 CI/release 阶段完成，目标机不安装 geoview，也不读取 DAT；
- Linux `auto_redirect`、nftables、pf；
- NetworkExtension provider；
- SmartDNS 独立进程；
- 应用自带 DoH/DoQ 的明文查询识别。

`macos/SteerApp`、`macos/SteerNetwork` 和 `macos/Package.swift` 保留为未来原生 UI/NetworkExtension 实验输入，但它们不是当前可交付运行时，也不应进入安装和 Apply 主路径。没有 Apple Developer Program 时，不声称它们可以安装或启动 provider。
