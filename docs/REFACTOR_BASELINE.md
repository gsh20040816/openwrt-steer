# M1 重构冻结基线

本文件冻结 Steer 下一阶段必须保留的产品能力、允许删除的历史实现和软件包所有权。它是
M1 重构的边界，不是对当前 ucode/SmartDNS/TPROXY 实现的长期承诺。

## 产品定位

Steer 是跨平台透明代理控制面和意图编译器。Steer 自己负责：

- Node、Route、Rule、DNS Profile 的统一语义；
- 严格校验、确定性编译和能力检查；
- 可检查的 Execution Plan；
- 原子 Apply、失败回滚和诊断；
- OpenWrt、普通 Linux 与 macOS 的薄平台适配层。

sing-box 负责代理协议、DNS transport、TUN 和实际转发。操作系统负责路由、防火墙和服务
生命周期。Steer 不实现代理协议、DNS resolver、TUN 协议栈或 eBPF 数据面。

## 冻结的功能

重构期间不得静默丢失以下已确认能力：

- 真实 IP；不使用 Fake-IP，不全局阻断 UDP/443；
- IPv4、IPv6、TCP、UDP、QUIC/HTTP3；
- 规则严格自上而下 first-match，没有隐藏 fallback；
- 一条规则显式选择 DNS Profile 与逻辑 Route；
- DNS 与业务 Route 的出口语义可以保持一致；
- 国内、国外、校园等 Split DNS；
- DNS 多上游容错以及 UDP、TCP、DoT、DoH、DoQ、DoH3 基本协议；
- VLESS、Hysteria2、Trojan 节点；
- Direct、Block、单节点逻辑出口；
- 域名、IP/CIDR、GeoSite、GeoIP、协议、端口、客户端和本地入口条件；
- 悬空引用、能力不足和非法配置显式失败；
- 原子 Apply、last-known-good 和失败回滚；
- 配置和决策可以被诊断，后续能够实现 `steer why`；
- OpenWrt LuCI 当前已覆盖的配置入口和单节点分享链接导入。

以上是语义冻结，不表示旧 UCI schema 或生成配置格式兼容。未发布 schema 可以破坏性升级，
但必须由校验器明确拒绝，不能静默解释成其他行为。

## 明确退出的实现

以下内容不是冻结功能，可以在对应替代路径通过测试后直接删除：

- SmartDNS 进程、配置语法、多实例生命周期和上游防回环 mark；
- Steer 自己维护的通用 TPROXY、策略路由和完整 nftables 数据面；
- ucode 中的平台无关模型、校验和编译逻辑；
- 路由器自行联网更新 GeoSite/GeoIP；
- SmartDNS 的 fastest-IP、主动测速和强制 TTL 下限；
- 仅用于历史实现的兼容字段和 fallback。

当前 SmartDNS 与 TPROXY 仍是迁移参考实现。只有新 sing-box DNS、TUN/auto_redirect 路径通过
专项回归与实机验证后，才删除对应旧路径；不长期维护双后端。

## M1 交付顺序

1. 固定包所有权，移除绕过包管理器的运行时更新。
2. 定义平台无关 semantic model 与 Execution Plan。
3. 将平台无关模型、校验和编译迁入 Go `steer-core`。
4. 将 DNS 切换为 sing-box DNS，并删除 SmartDNS 专属语义。
5. 在 OpenWrt 验证 TUN、`auto_route`、`auto_redirect` 和必要的小型 nftables shim。
6. 覆盖 LAN/本机、双栈、TCP/UDP/QUIC、DNS、回环、reload 和 rollback。
7. 稳定运行 2～4 周后再开始普通 Linux adapter；macOS 在其后验证。

## 当前不做

- 自研或 fork sing-box；
- 自研 DNS daemon、TUN/UDP NAT 或 eBPF/XDP 数据面；
- 大量 Clash 兼容、订阅脚本 DSL、provider 或 dashboard 生态；
- 为尚无真实用例的高级能力编写运行时抽象；
- 在 M1 未完成前并行开发 Linux/macOS UI。
