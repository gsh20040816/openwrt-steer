# ADR 0002：macOS 使用 LaunchDaemon + sing-box TUN

## 状态

Accepted；这是 macOS 唯一受支持的数据面架构。

## 背景

项目不购买 Apple Developer Program。sing-box 已经提供 macOS TUN 和
`auto_route`，Steer 不需要重新实现数据面。SwiftUI GUI 只作为配置与运维
前端，数据面继续由 root LaunchDaemon 管理。

## 决策

macOS 使用普通 sing-box 二进制，由 root LaunchDaemon 启动：

```text
steer-macos apply
        ↓
launchctl bootstrap
        ↓
steer-macos _run
        ↓
exec sing-box run -c current/sing-box.json
        ↓
Darwin utun + auto_route
```

平台 plan：

- TUN inbound 不设置 `interface_name`，让 Darwin/utun 自动选择设备；
- 设置 `auto_route`、`stack=system`、MTU 和地址；
- 使用 `route_exclude_address` 保留本机/局域网/非全球地址；
- 不设置 Linux-only `auto_redirect`、iproute2 mark、nftables 或 pf。

DNS 由同一份 sing-box DNS Router 负责。TUN inbound 上只匹配 TCP/UDP 目标端口 53，再执行 `hijack-dns`；普通 UDP session 不匹配该规则。

## 生命周期

`steer-macos apply` 通过同步 backend 完成：

```text
Validate → Compile → sing-box check → generation prepare
       → launchctl bootout old
       → atomic publish current.json
       → launchctl bootstrap new
       → check launchd + utun addresses
       → prune old generations
```

LaunchDaemon 使用 `RunAtLoad=true`、`KeepAlive=false`。禁用时由 `cleanup`/Apply unload 服务并删除 current generation，避免 disabled 配置造成 launchd 重启循环。

## 不做的事

- 不把 Swift GUI 放入数据面或 Apply 编译路径；
- 不引入 SmartDNS；
- 不让 macOS 复用 Linux 的 forwarded DNS PREROUTING 逻辑；
- 不让 GUI 绕过 helper 直接写 generation 或启动 sing-box。
