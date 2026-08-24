# macOS targets

macOS 的当前正式运行时不使用 NetworkExtension，也不要求 Apple Developer Program。主路径是：

```text
steer-macos
├── validate / compile / prepare
├── apply / health / status / cleanup
└── _run → exec sing-box

macos/launchd/com.gsh20040816.steer.plist
└── root LaunchDaemon
```

当前已提交的运行时输入位于：

- `../go/internal/platform/macos/`：Darwin TUN、DNS port-53 capture、generation 和 launchd backend；
- `launchd/`：LaunchDaemon plist；
- `scripts/install-launchdaemon.sh`：构建 helper、发现 sing-box、安装并 bootstrap 服务；
- `../go/cmd/steer-macos/`：macOS CLI。

## 安装和运行

先安装外部 sing-box：

```sh
brew install sing-box
```

再执行：

```sh
sudo macos/scripts/install-launchdaemon.sh
sudo /usr/local/libexec/steer/steer-macos validate
sudo /usr/local/libexec/steer/steer-macos apply
sudo /usr/local/libexec/steer/steer-macos health
```

安装脚本会根据 `command -v sing-box` 自动处理 `/opt/homebrew/bin` 和 `/usr/local/bin`，不把某个 Homebrew 前缀写死到运行逻辑中。

## TUN 和 DNS

sing-box 负责 Darwin utun 和 `auto_route`；macOS plan 不设置 `auto_redirect`、nftables、pf 或 Linux mark。DNS 由 sing-box 内部 DNS Router 处理，Steer 只生成明确匹配 TUN 上 TCP/UDP 53 的 `hijack-dns` route rule。

Geo 不是 macOS 语义限制。若 Intent 使用 `geosite:`/`geoip:`，把与当前 master 构建
匹配的 `geodata-seed/`（含 `manifest.json` 和 `rules/*.srs`）放入
`/Library/Application Support/Steer/geodata-seed/`。Apply 会按 manifest 校验所需 SRS；
目标机不安装 geoview，也不读取 DAT。

## Swift / NetworkExtension（未来实验）

`SteerApp/`、`SteerNetwork/` 和 `Package.swift` 保留为未来原生 UI/NetworkExtension 实验输入，但不是当前正式运行时。没有付费 Apple Developer 账号时，不应配置、安装或声称 Packet Tunnel/DNS Proxy provider 可用；它们也不参与 LaunchDaemon 安装和 Apply。
