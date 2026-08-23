# ADR 0001：macOS DNS 先捕获、再进入专用 inbound

- 状态：accepted
- 日期：2026-08-24
- 范围：macOS NetworkExtension 与共享 sing-box 编译目标

## 决策

macOS 使用 `NEDNSProxyProvider` 作为第一层 DNS 捕获器，捕获传统 UDP/TCP 53 flow 后，转发到生成配置中的专用 loopback DNS inbound。该 inbound 继续使用 sing-box `route.action = "hijack-dns"` 进入 DNS Router。

Packet Tunnel 的普通 UDP session 不得直接匹配 `hijack-dns`，Packet Tunnel 也不安装 `NEDNSSettings`。DNS Proxy 是系统 DNS 捕获的唯一所有者。

## 原因

Linux/OpenWrt 已经采用“先按目标端口分流，再进入专用 inbound”的两层结构。这里的 `hijack-dns` 处理的是已经确认的 DNS 流，而不是让 TUN UDP session 猜测后续数据包是否为 DNS。macOS 复用这个边界可以保留 sing-box 的 DNS Profile、Route、缓存、detour 和上游协议实现，同时避免把 Apple TUN 误当成 Linux `auto_redirect` 环境。

## 约束

- 必须同时覆盖 UDP 与 TCP DNS；
- TCP 必须处理两字节长度前缀、半包、粘包、多 query、超时和取消；
- DNS Proxy 到 DNS inbound 的 generation ID 必须与 Packet Tunnel 一致；
- provider 出口不能重新进入 Packet Tunnel；
- 应用自行使用 DoH/DoQ 时没有明文 DNS 可供 Proxy 捕获，不做 TLS MITM，也不宣称能识别这类查询；
- 未经真实 Mac 签名和系统授权验证，不得把 provider 激活状态写成 healthy。

## 后果

共享 compiler 只需要通用的 `DNSCapture` 合同；平台适配器负责第一层捕获，Swift 不重写 DNS 解析和规则选择。未来若改用不同的捕获传输，只需替换 provider/bridge，不改变 Canonical Intent 或 UI 语义。
