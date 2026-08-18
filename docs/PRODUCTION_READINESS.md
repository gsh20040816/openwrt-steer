# Steer 生产就绪审计

> 审计日期：2026-08-17
>
> 当前结论：允许以预览版软件包安装到生产路由器，但必须保持 UCI 与 init 双重停用；
> 尚不允许接管生产流量。

## 1. 审计目的

本审计不按其他透明代理工具的菜单数量追功能。对照重点是会导致断网、策略外直连、
回环、重启后失效或无法恢复的系统边界。功能丰富但语义隐含的实现只作为风险样本，
不会直接复制进 Steer。

本轮按以下上游提交核对实际代码：

- [OpenWrt-momo](https://github.com/nikkinikki-org/OpenWrt-momo/tree/72f5c46b5b65ad95f8f786f024c98204e47cd3dd)：
  `72f5c46b5b65ad95f8f786f024c98204e47cd3dd`；
- [HomeProxy](https://github.com/immortalwrt/homeproxy/tree/edece28a0085f36d469ec82c8d45f562f602db53)：
  `edece28a0085f36d469ec82c8d45f562f602db53`；
- [PassWall2](https://github.com/Openwrt-Passwall/openwrt-passwall2/tree/8916e7fe55354771bfc22f854c328db5a9887ec3)：
  `8916e7fe55354771bfc22f854c328db5a9887ec3`；
- [OpenClash](https://github.com/vernesong/OpenClash/tree/c3a33c1d3407956fdf8f0e0b7c1a4c52e6ad9593)：
  `c3a33c1d3407956fdf8f0e0b7c1a4c52e6ad9593`。

## 2. 从上游确认的生产问题域

### 2.1 流量方向和核心防回环

Momo 与 PassWall2 都在路由器 output 接管中排除 conntrack reply，避免本地服务的回包
再次进入透明代理。Momo 还允许按 cgroup、UID、GID、mark 和 DSCP 表达核心或系统进程
旁路；PassWall2/OpenClash 则持续维护节点地址集合。Steer 依靠 sing-box 出站 routing mark
避免核心回环，这个模型更简单，但仍必须排除 reply，并验证所有核心发出的 DNS 与协议
连接确实携带绕行 mark。

### 2.2 网络和接口生命周期

HomeProxy 注册 WAN interface-up trigger，PassWall2 在 ifup/ifupdate 时更新 WAN 集合或重启，
OpenClash watchdog 会刷新 WAN/LAN 地址并在防火墙变化后恢复规则。Steer 当前只在生成运行代
时把受管 zone 解析成设备名；接口重建后 firewall4 仍可能恢复旧设备集合，新设备流量可能
绕过接管。

### 2.3 核心启动、崩溃和防火墙顺序

Momo 在 TUN 模式等待设备上线后才装载接管；HomeProxy 和 OpenClash 都先执行核心配置检查，
OpenClash 另有 watchdog 与核心状态检查。Steer 已在 Apply 前执行 sing-box 与 nftables 原生
检查，并在新代不健康时回滚，但开机启动路径没有执行同等的健康事务，也不会在当前 UCI
损坏时自动选择 last-known-good。

### 2.4 进程权限和资源边界

HomeProxy 在可行时以 `sing-box` 用户、capability allowlist、`no_new_privs` 和 ujail 运行核心，
并设置文件描述符上限。Steer 当前所有核心与 DNS 实例均以 root 运行，也没有显式资源上限。
这不直接改变路由语义，但扩大了核心或解析器漏洞的影响范围。

### 2.5 订阅、更新和可恢复状态

Momo、HomeProxy、PassWall2 与 OpenClash 都把订阅获取、定时更新和本地缓存当成独立生命周期。
Steer 已对 GeoSite/GeoIP 实现候选校验和 last-known-good，但尚无订阅对象、稳定节点身份、
部分更新失败语义或订阅回滚。发布为生产可用版本前必须补齐这条生命周期。

## 3. 已确认并修复的遗漏

- Bootstrap DNS 使用 IP 字面量和核心绕行 mark，启动不依赖代理核心；
- 路由器本机 UDP/123 NTP 固定直连，LAN 客户端 UDP/123 仍进入普通 Rules；
- SmartDNS 业务上游只跳过本机 53 再劫持，之后仍按普通 Rules 路由；
- 路由器 output 排除 conntrack reply，不误代理本地服务回包；
- 受管 zone 按实际 ingress 设备匹配，不按“私有源地址集合”猜测 LAN 身份；
- 双栈 DNS 与 TPROXY 使用同一接管边界，不默认阻断 UDP/443；
- 配置候选通过 sing-box 与 nftables 原生检查后才切换，并保留 last-known-good；
- 已知 PassWall2、OpenClash、HomeProxy、Momo 同时启用或运行时拒绝 Apply。

## 4. 生产接管前的 P0 阻塞项

### P0-1：把故障关闭做成显式状态机

维护者已选择：核心或任一 DNS Profile 持续不可用时保持接管并阻断受管 WAN，不撤销规则
形成策略外直连；Bootstrap DNS、系统 NTP、本地管理面和非全局目标保持可用。

当前实现只是核心退出后留下 TPROXY 规则，结果表现为连接超时，并没有显式的 `blocked` 运行
状态、原因和可验证阻断链。必须增加监督状态、日志和 LuCI 告警，并覆盖核心崩溃、DNS 实例
崩溃、respawn 耗尽和二进制丢失。

### P0-2：开机必须复用 Apply 的事务语义

失败 Apply 会保留当前运行代，但用户新写入的 UCI 仍留在磁盘。路由器重启后 `/var/run`
消失，init 会重新编译损坏的 UCI，当前不会自动启动 last-known-good。必须做到：

1. 开机先校验当前 UCI；
2. 当前 UCI 无效或启动不健康时尝试 last-known-good；
3. 两者都失败时进入 P0-1 的显式故障关闭状态；
4. 状态页区分“运行 LKG”和“当前期望配置”；
5. 不把失败配置静默覆盖，以便用户修正。

### P0-3：统一 UCI enabled 与 init 自启动

当前 `option enabled '1'` 能让 Apply 启动服务，却不会执行 `/etc/init.d/steer enable`；当前会话
看似成功，重启后可能不启动。启用事务必须在健康检查成功后设置 init 自启动；停用事务必须
先撤销网络接管，再禁用 init。软件包首次安装仍必须保持两者均为停用。

### P0-4：处理接口热插拔和 zone 设备变化

必须监听与受管 zone 有关的 ifup、ifupdate、ifdown 和 firewall reload，重新解析实际设备并
原子替换仅网络层规则。不得在接口尚未出现时把空设备集当成成功，也不能因一次短暂 WAN
变化重启所有 DNS 缓存。需验证 IPv6 PD 更新、桥成员变化和 Guest/IoT zone 重建。

### P0-5：检查系统资源冲突和规则所有权

现有模型只检查 Steer 配置内部端口与 mark 冲突。Apply 还必须检查：

- TPROXY、DNS Profile、本地代理监听端口已被其他进程占用；
- rule priority、route table 与 fwmark/mask 已被非 Steer 规则使用；
- `inet steer` nftables 表是否确由本实例拥有；
- 已运行但未列入名称清单的透明代理或手写规则；
- 所需内核模块、fw4、sing-box、SmartDNS 和 geoview 的实际版本。

冲突必须在替换当前运行代前失败，不能先删别人的规则再报错。

### P0-6：实现订阅最小闭环

第一版不需要兼容所有订阅格式，但已声明支持的 VLESS、Hysteria2 与 Trojan 必须进入同一订阅生命周期：

- 订阅地址与凭据 root-only 保存，日志和 RPC 默认脱敏；
- 下载使用 Bootstrap DNS 和明确路由语义；
- 远端节点使用稳定本地身份，更新不能让 Rules/Route 引用悬空；
- 新订阅先解析、校验、生成候选并原子替换，失败保持上一版本；
- 节点删除、同名、协议变化和部分成功必须显式报告；
- 订阅更新造成的悬空引用必须阻止应用，不能自动落入 Default。

### P0-7：完成真实数据面和冲突测试

现有 KVM 已验证控制面、路由器本机 TCP/UDP/DNS/NTP 和生成规则，但还必须补齐：

- 来自受管 LAN 的 IPv4/IPv6 TCP、普通 UDP、QUIC/HTTP3；
- LAN GUA 发往公共 IPv4/IPv6 的 UDP/TCP 53；
- 源 MAC 规则对同一客户端的 IPv4、IPv6、SLAAC、临时地址和传统 DNS 保持同一 DNS/Route 决策；
- LAN 客户端 UDP/123 不命中系统 NTP 旁路；
- VLESS Reality/Vision、Hysteria2 和 Trojan 的真实远端连接；
- 核心/DNS 崩溃、开机坏 UCI、LKG 回退、firewall reload、接口重建；
- 代表性目标硬件上的长期运行、内存压力和 flow offload 隔离。

## 5. 发布与安装阻塞项

GitHub 预览版发布前至少还要完成：

- 补齐 GPL-3.0 许可证正文和来源清单；
- 用 OpenWrt 25.12.5 x86/64 SDK 构建 `.apk`，锁定 geoview 与 geodata 输入；
- 修正并验证独立 feed 中的 LuCI 构建路径；
- 发布 SHA-256，保留构建日志，并把不满足完整依赖组合标为失败；
- 在干净 KVM 安装实际发布资产，确认 UCI `enabled=0`、init 未启用、无进程、无 `inet steer`
  表、无 Steer policy rule；
- 增加卸载清理，不能留下失效 firewall include、运行进程或策略路由。

生产路由器只允许安装上述已验证的预览包，不允许启动或接管。安装前后必须记录现有网络服务、
firewall4 状态和 Steer 的双重停用状态。安装预览包不等于生产切换批准。

## 6. 非 P0 能力

以下能力仍有价值，但不会阻止第一版最小闭环：

- TUN/Redirect 等第二种透明接管模式；
- 加权负载均衡、一致性哈希和复杂主动健康策略；
- 任意布尔规则 DSL；
- 广泛设备架构与旧 OpenWrt 版本适配；
- 自动迁移其他代理插件的历史配置；
- OpenClash 式任意核心配置与脚本覆盖入口。

这些能力只有在出现真实需求并能保持“指哪打哪”时才进入开发计划。
