# M1 破坏性重构冻结基线

本文件是 Steer M1 的唯一权威架构边界。旧 SmartDNS、通用 TPROXY、schema 3、boot LKG、
多 DNS 上游和独立 `steer-core` 包设计均已退出，不构成兼容要求。

## 决策优先级

1. 正确性；
2. 简洁、确定且可解释的语义；
3. 可验证性；
4. 可维护性；
5. 性能；
6. 兼容性。

M1 允许破坏旧配置、包名和内部生成格式。非法配置、未知字段、悬空引用、能力不足、WAN
歧义和资源冲突必须在修改系统前失败。不得通过兼容读取、隐藏 fallback、自动迁移、双后端或
静默降级让失败看似成功。

失败不是正常运行模式。Steer 只实现保证配置提交正确所需的最小事务，不建设通用自愈、历史
代管理、自定义 watchdog 或软件包自动降级系统。

## M1 范围

M1 只交付 OpenWrt 25.12 adapter。Canonical Intent、校验器、编译器和 Execution Plan 必须
保持平台中立；普通 Linux 与 macOS adapter 只能在 OpenWrt M1 真机验收完成后开始。

Steer 是透明代理意图编译器和控制面，不实现代理协议、DNS resolver、TUN 栈或 eBPF 数据面。
sing-box 负责 DNS transport、代理协议、TUN 和实际转发；OpenWrt 负责 UCI、procd、firewall4
与网络生命周期。

## 运行拓扑

OpenWrt 只安装一个非驻留 Go 控制程序 `/usr/sbin/steer`。它负责：

- 严格 UCI 解码；
- Canonical Intent 校验；
- sing-box 配置与平台 Execution Plan 编译；
- 候选与当前代的语义差异；
- 能力、资源和原生配置检查；
- 事务 Apply 与状态查询。

procd 直接监督发行版 `/usr/bin/sing-box`。Steer 不常驻，不再增加一层子进程监督协议。

## 上游基线

支持的 sing-box 范围固定为：

```text
>= 1.13.18
<  1.14.0
```

不能仅按 `sing-box`/`sing-box-tiny` 包名判断能力。Apply 必须读取实际版本和 build tags，并按
当前 Intent 所需能力失败关闭。生成配置还必须经过同一二进制的原生 `check`。

Steer 不 fork、不复制、不替换 sing-box。后续支持 1.14 必须作为显式能力基线变更处理，不能
把两个配置后端暴露给用户。

## schema 6

公开 UCI 只包含以下对象：

- `steer`；
- `bootstrap`；
- `node`；
- `subscription`；
- `route`；
- `dns_profile`；
- `local_proxy`；
- `rule`。

规则引用 `route`，不再使用 `outbound`。一个 DNS Profile 直接拥有一个上游，不再有独立
`dns_server` 对象或上游列表。用户配置不得包含 TUN 地址、内部监听端口、mark、route table、
nft table 或 procd 参数；这些资源由 OpenWrt adapter 确定分配并在 Plan 中显示。

Node、Route、DNS Profile、Local Proxy 和 Rule 保留统一 `enabled` 语义：禁用对象不进入
编译；启用对象引用禁用对象是硬错误；未引用对象允许存在。所有 section ID 共享一个全局
命名空间。

schema 4 及更早版本不迁移、不猜测、不兼容。用户在升级 APK 前人工完成生产配置迁移；旧
schema 必须保留原文件并拒绝启动。

## 冻结产品语义

- 真实 IP；不使用 Fake-IP；
- IPv4、IPv6、TCP、UDP、QUIC/HTTP3；
- 不通过全局阻断 UDP/443 强迫降级；
- 路由器自身流量始终与受管客户端进入同一规则体系；
- 规则严格自上而下 first-match；
- 每条启用规则显式选择一个 DNS Profile 和一个 Route；
- DNS 投影只使用查询阶段可观察条件，Route 投影使用完整连接条件；
- 同字段多个值为 OR，不同非空字段为 AND；
- 末尾必须恰好有一个启用且不可带匹配条件的 Default；
- Domain、IP/CIDR、GeoSite、GeoIP、TCP/UDP、嗅探协议、端口、源 IP、源 MAC 与 Local
  Proxy inbound 条件；
- VLESS、Hysteria2、Trojan；
- Direct、Block、单节点 Route；
- UDP、TCP、DoT、DoH、DoQ、DoH3；
- 具名 SOCKS、HTTP、mixed 本地代理入口；
- LuCI 单节点分享 URL 的严格、脱敏导入；
- package-owned GeoSite/GeoIP 数据及本地 `.srs` 派生；
- CLI/LuCI 的 Plan、语义差异、校验、Apply 和关键状态。

当前重构在上述 M1 数据面之外补交付了标准 URI/Base64 订阅、UCI 批量节点更新、定时调度、失联节点显式清理和临时节点测速；多节点故障转移、高级规则 DSL 与 `steer why` 仍不交付。

## DNS

一个 DNS Profile 只能配置一个上游，没有自动后备链。上游失败不得落入另一个 Profile 或
系统 resolver。

编译器为每个被启用规则实际引用的 `(DNS Profile, Route)` 组合创建独立 sing-box DNS
transport，并将 Route 固定为该 transport 的 detour。sing-box 1.13 必须生成
`independent_cache: true`，使缓存按实际 transport 隔离。

schema 预留持久 DNS cache 和 optimistic cache 字段。在 1.13 基线上启用这些字段必须以能力
错误失败，不能忽略或模拟。

传统 LAN 与路由器 TCP/UDP 53 使用一个最小 nft redirect shim 进入专用 sing-box direct DNS
inbound，不依赖 TUN `hijack-dns` 对 UDP 会话的首次分类。未来原生机制替代后直接删除 shim，
不迁移用户 schema。

## 透明数据面

普通流量统一使用 sing-box TUN、`auto_route`、`strict_route` 和 `auto_redirect`。Steer 不再
维护通用 TCP/UDP TPROXY、通用策略路由或完整 nftables 数据面。

sing-box 1.13 不能原生匹配源 MAC。M1 允许唯一的临时兼容实现：nft 在受管二层 ingress 按
`ether saddr` 把该 MAC 的 DNS 和普通 TCP/UDP 分别送入专用 sing-box DNS/TProxy inbound。
它不得扩展成通用 TPROXY 后端，不做 MAC→IP 邻居解析。1.14 原生 MAC 能力通过测试后，应在
不改 schema 的情况下删除该 shim。

平台 adapter 必须在 Apply 前确认唯一默认 WAN、受管 zone 的实际设备、内部端口、TUN 名、
mark、table、priority 和 nft table 均无歧义或冲突。

## Apply

LuCI 的“保存”只写会话暂存；“保存并应用”通过会话化 ubus `uci commit` 写入磁盘。该提交会
发送 OpenWrt 标准 `config.change` 事件，由 procd 的 `reload_service` 恰好启动一次
`steer apply`。LuCI 只等待并显示这次 Apply 的结果，不得再调用第二个 Apply RPC。

裸 `/sbin/uci commit steer` 本身不会发送 ubus 事件，Steer 不包装系统 `uci`，也不为此增加
常驻文件监视器。终端的受支持入口是
`uci commit steer && /etc/init.d/steer reload`；init reload 与 LuCI 触发共用同一个 Apply。

Apply 固定流程：

1. 严格解码、校验、能力和资源检查；
2. 生成候选 Plan、语义差异、sing-box JSON 和平台产物；
3. 执行 sing-box 与 nftables 原生离线检查；
4. 如果当前运行代通过本地健康检查，把它的 UCI 覆盖到唯一持久恢复点；
5. 串行停止当前实例并启动候选；
6. 验证进程、TUN、路由、nft table 和监听器；
7. 本地健康后提交并清理旧临时生成代。

所有 Apply 使用 `/run/steer/apply.lock` 的阻塞文件锁串行。连续提交不保存历史候选快照；每次
事件执行时读取最新已提交配置。即使 Intent digest 没有变化也执行完整 Apply，不增加 no-op
分支。

配置解码、校验或离线预检失败发生在停止当前实例之前：候选保留，当前运行代不变。候选开始
切换后若启动或本地健康检查失败，Apply 立即失败并保留故障现场，不自动停止、不自动恢复
UCI 或运行代。失败不是预期运行模式，不为它维护双向状态交换和自动恢复控制流。

`/var/lib/steer/rollback.uci` 只保存切换前最后一个本地健康运行代的 UCI。`steer rollback`
恢复这份 UCI 并复用同一 Apply；成功后一次性删除备份，失败则保留。LuCI 在备份存在时显示
带确认的恢复按钮，并调用同一个命令。它不是多版本历史或自动 LKG。

HTTPS probe 已退出 Apply，只能由用户显式运行 `steer probe`。该命令针对当前 Plan 并行请求
配置的 HTTPS URL，每个目标最多尝试两次，要求有效 TLS 和 2xx/3xx；结果只用于诊断，不改变
运行态。

候选切换允许短暂网络中断，不实现双实例无缝接管。普通 boot/restart 只重新编译已提交配置
并做本机就绪检查。

不保存历史运行代或 boot LKG。重启时配置非法就拒绝启动。运行期间 sing-box 崩溃不触发配置
回滚，只使用 procd 默认 respawn；不得把这种状态描述为 fail-closed。

## 包布局与升级

- `steer-openwrt`：Go 控制程序、UCI、init、firewall4 和 OpenWrt adapter；
- `luci-app-steer`：依赖 `steer-openwrt`；
- `steer-geodata`：固定版本和哈希的源 Geo 数据；
- `geoview`：独立转换工具；
- 发行版 `sing-box`：上游二进制。

不创建独立 `steer-core` APK。`steer-openwrt` 必须通过包管理器正确 replace/conflict 旧
`steer`。切包时停止旧服务并保留 `/etc/config/steer`；只有 schema 6 preflight 通过才启动新
服务。失败时不恢复默认配置、不运行旧后端、不自动降级 APK。当前 schema 5→6 是唯一迁移窗口。

## LuCI

LuCI 与后端同步破坏式重写，必须能编辑全部公开 schema，并显示 Validation、Execution Plan、
语义差异、Apply 结果和关键运行状态。旧 SmartDNS、TPROXY、mark 与 router traffic 开关页面
直接删除。

LuCI RPC 只能调用固定的 Steer 命令，不获得任意 shell 权限。分享 URL 仍在浏览器本地解析，
不能静默丢弃未知传输或安全参数。

## 删除条件

在新路径进入 alpha 前必须删除：

- SmartDNS 依赖、配置、进程和测试；
- 通用 TPROXY/firewall ucode/策略路由实现；
- 平台无关 ucode model/compiler；
- shell runtime 事务；
- schema 3 默认配置和 fixtures；
- boot LKG、运行时 updater、兼容字段和旧 LuCI 页面。

开发期间新旧源码可以短暂同时存在，但任何可安装版本都不得暴露运行时后端选择。切换包、
init、RPC 与 LuCI 时必须同时删除旧运行入口。

## M1 放行门

发布 alpha 前，隔离 VM/network namespace 必须全部通过：

- schema、能力、资源和确定性编译；
- 三种节点协议、六种 DNS transport；
- 双栈 LAN/路由器 TCP、UDP、QUIC；
- Direct、Block、单节点 Route；
- 所有普通规则条件及 DNS/Route 投影；
- DNS 路径与缓存隔离；
- TCP/UDP 53 shim、DNS source-port 复用后 STUN 不误劫持；
- MAC 双栈 shim；
- Direct、代理 outbound 与核心自身无 TUN 回环；
- Geo 首次生成、数据升级和缺失分类失败；
- Apply 成功、失败现场保留、单份健康 UCI 手动恢复、HTTPS 手动诊断；
- fw4/network reload、reboot、procd 连续失败；
- 旧 `steer` 包到 `steer-openwrt` 有效/无效 schema 包升级；
- 无 Fake-IP、无 UDP/443 阻断、无隐藏 fallback、无 JSON 逃生口；
- CPU、内存、日志和吞吐数据记录；环路、泄漏、无界增长是硬失败。

隔离门全过后发布 alpha。生产路由器只验证成功路径，不注入故障；必须在真实 IPv4/IPv6 上
完成实际节点、DNS、规则、三站 Apply、TCP/UDP/QUIC、Split DNS、Direct/Proxy/Block、当前
使用的 MAC/Local Proxy、fw4/network reload 与 reboot。真机清单通过后 M1 才完成，随后才能
开始 Linux adapter。
