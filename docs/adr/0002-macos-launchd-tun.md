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
- 静态把 IPv4 RFC1918、CGNAT 与 IPv6 ULA 全部加入 TUN `route_address`；
- `route_exclude_address` 继续保留回环、链路本地、组播和文档/保留地址，但不再排除上述私网；
- 不设置 Linux-only `auto_redirect`、iproute2 mark、nftables 或 pf。

DNS 由同一份 sing-box DNS Router 负责。TUN 使用 `dns_mode=disabled`，不修改系统 DNS；route 的第一条规则严格匹配 `inbound=steer-tun + network=[tcp,udp] + port=[53]` 后执行 `hijack-dns`。`port` 是目标端口，规则禁止使用 `source_port` 或 `protocol=dns` 嗅探。第二条规则将 RFC1918、CGNAT 与 ULA 固定路由到 Direct，之后才是 sniff 和用户公网规则。因此这些范围内的 DHCP DNS 与应用硬编码 Do53 都先被截获；其他私网单播流量会经过用户态核心但保持 Direct。DoH/DoT 不属于端口 53 劫持范围。普通 global IPv6 on-link 地址不作为私网特殊处理。

plan 完全由静态平台常量和 Canonical Intent 决定。control LaunchDaemon 不监控 LAN prefix；接口、Wi-Fi 和 DHCP 变化不会重新生成配置，更不会把仅保存但尚未 Apply 的业务修改带入运行态。

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
