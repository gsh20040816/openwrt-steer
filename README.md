# Steer

Steer 是面向 OpenWrt 的透明代理意图编译器和控制平面。它把节点、逻辑出口、DNS Profile 与有序规则编译为受控的 sing-box 配置，并由 OpenWrt 原生的 procd、firewall4 和 UCI 管理运行状态。

当前仓库处于第一版开发阶段，目标是提供语义清晰、可验证的 OpenWrt 透明代理控制面，而不是复刻其他插件的全部功能。

当前 UCI schema 为 3。schema 1 的内部 DNS SOCKS 模型和 schema 2 的拆分匹配字段均已删除；旧配置会明确拒绝编译，不会静默忽略遗留语义，也不会为尚未正式发布的旧 schema 继续增加兼容层。

## 第一版边界

当前实现中的首批不变量：

- 规则从上到下执行，第一条命中后停止；
- SmartDNS 业务上游是路由器本机流量，与其他本机连接共用普通规则；
- 每条启用规则同时引用一个 DNS Profile 和一个逻辑出口；
- 只能存在一条启用的 Default；LuCI 将它固定在所有普通规则之后，只允许选择 DNS Profile 和 Route；
- 启用对象出现悬空引用时拒绝编译，不能静默落入 Default；
- 禁用规则保留但不进入生成配置；
- 启用透明代理时必须明确选择受管 firewall zone；
- Bootstrap DNS 必须使用 IP 字面量并携带核心绕行 mark 直连；其他 SmartDNS 上游只携带一个不与 TPROXY mask 重叠的防回环 mark，跳过本机 53 端口再劫持后仍进入 output TPROXY 和普通规则；
- 路由器本机发起的 UDP/123 NTP 是启动依赖，固定直连且在 Rules 页显示命中计数；LAN 客户端的 UDP/123 不享受该例外，仍进入普通规则；
- 不生成 Fake-IP，也不生成全局 UDP/443 或 QUIC 阻断规则；
- 首批节点协议包括 VLESS、Hysteria2 与 Trojan；
- LuCI 在浏览器本地解析单个 `vless://`、`hysteria2://`、`hy2://` 或 `trojan://` 分享链接，先显示不含凭据的审查结果再写入待提交 UCI；无法由当前节点模型保留的传输或安全参数会明确拒绝，不会静默丢弃；
- 首批逻辑出口只支持单节点、直连和阻断；连接失败后的自动切换尚未实现。
- 混合规则的 DNS 投影只使用查询阶段可见字段，Route 投影保留目标 IP、端口和 TCP/UDP；只有连接阶段条件时不会生成空 DNS 规则。
- GeoSite/GeoIP 使用 Loyalsoldier 数据发布，定期转换为受控本地 sing-box 规则集；数据源不能携带 DNS、出口或 fallback 动作。

更完整的设计、审计证据与开发边界见 [项目规划](docs/PROJECT_PLAN.md)；跨工具生命周期对照、
生产阻塞项和“仅可停用安装”的边界见 [生产就绪审计](docs/PRODUCTION_READINESS.md)。

## 仓库结构

- `steer/`：OpenWrt 后端包、UCI 配置与 ucode 编译器；
- `luci-app-steer/`：LuCI 应用、独立的简体中文语言包与最小权限 RPC；
- `tests/`：语义模型、回归与目标机验证；
- `docs/`：规划、上游选择与公开的工程验证记录。

## 当前状态

当前后端已建立配置模型、严格校验、确定性编译和首个可运行的 OpenWrt 接管闭环。它能由 procd 启动一个 sing-box 与多个 SmartDNS 实例，按受管 firewall zone 的实际设备加载双栈 DNS/TPROXY 规则，并在 Apply 前执行原生检查。路由器本机 TCP、UDP 与传统 DNS 可由同一总开关进入普通 Rules；IANA 非全局可达目标在核心前旁路。GeoSite/GeoIP 使用版本锁定种子、独立源文件更新和 last-known-good 生成代；规则编辑器在合并后的域名和目的 IP 字段中动态补全合法分类，后端自动生成 sing-box 本地规则集，并用显式 logical OR/AND 固化“字段内 OR、字段间 AND”。首版 LuCI 已覆盖总览、普通规则、具名本地代理、DNS Profile、DNS Server、节点和逻辑出口，并通过专用 RPC 执行同一事务式 Apply。Nodes & Routes 页面还能在浏览器本地解析当前支持协议的单节点分享 URL；这不等于订阅 URL 导入或更新。更多故障注入和生产切换仍未完成，因此还不能称为生产可用版本。

DNS 的对象关系固定为：普通规则选择 DNS Profile；一个 DNS Profile 对应一个由 Steer 管理的 SmartDNS 实例；SmartDNS 业务上游作为路由器本机连接进入同一套普通 Rules。DNS Server 自身没有出口字段，避免出现与规则并行的隐式路由语义。

当前后端提供以下主要命令：

- `steerctl validate`：输出结构化错误和警告；
- `steerctl compile`：输出完整编译 bundle；
- `steerctl compile-sing-box`：只输出 sing-box JSON；
- `steerctl compile-smartdns <profile-id>`：只输出指定 SmartDNS 实例配置。
- `steerctl compile-firewall <device>...`：为已解析的 ingress 设备生成 nftables 规则；
- `steerctl apply`：生成候选、执行 sing-box/nftables 原生检查、切换 procd 运行代并在本地健康检查通过后保存 last-known-good。

SmartDNS 当前没有可依赖的纯配置检查接口，因此 Apply 不会伪称已对它完成离线原生检查；SmartDNS 配置由严格编译器生成，并在切换后检查各 procd 实例是否持续运行。失败时恢复上一运行代。公网可达性不参与自动回滚。

## 许可证

Steer 以 GPL-3.0-or-later 发布，许可证全文见 [LICENSE](LICENSE)。第一版运行集成参考
OpenWrt-momo 的工程结构，但 Steer 使用自己的语义模型和实现；其他上游只作为行为与工程
参考。SDK、geoview、GeoSite/GeoIP 等直接发布输入及固定哈希见
[发布输入与来源](docs/SOURCES.md)。
