# Steer

Steer 是面向 OpenWrt 的透明代理意图编译器和控制平面。它把节点、逻辑出口、DNS Profile 与有序规则编译为受控的 sing-box 配置，并由 OpenWrt 原生的 procd、firewall4 和 UCI 管理运行状态。

当前仓库处于第一版开发阶段，目标是提供语义清晰、可验证的 OpenWrt 透明代理控制面，而不是复刻其他插件的全部功能。

项目已进入 M1 破坏性重构。必须保留的产品能力、允许退出的历史实现和开发顺序以
[M1 重构冻结基线](docs/REFACTOR_BASELINE.md)为准；软件包与文件所有权以
[打包与文件所有权](docs/PACKAGING.md)为准。旧 SmartDNS、通用 TPROXY、schema 3 和 shell
runtime 已删除，不构成兼容接口。

当前 UCI schema 为 4。旧配置会明确拒绝编译，不会静默迁移或忽略遗留语义。

## 第一版边界

当前实现中的首批不变量：

- 规则从上到下执行，第一条命中后停止；
- 每条启用规则同时引用一个 DNS Profile 和一个逻辑出口；
- 只能存在一条启用的 Default；LuCI 将它固定在所有普通规则之后，只允许选择 DNS Profile 和 Route；
- 启用对象出现悬空引用时拒绝编译，不能静默落入 Default；
- 禁用规则保留但不进入生成配置；
- 启用透明代理时必须明确选择受管 firewall zone；
- Bootstrap DNS 必须使用 IP 字面量直连；每个实际使用的 `(DNS Profile, Route)` 都编译为独立 sing-box DNS transport；
- 路由器本机与受管客户端进入同一规则体系；传统 TCP/UDP 53 由最小 nft redirect shim 送入 sing-box DNS；
- 不生成 Fake-IP，也不生成全局 UDP/443 或 QUIC 阻断规则；
- 首批节点协议包括 VLESS、Hysteria2 与 Trojan；
- LuCI 在浏览器本地解析单个 `vless://`、`hysteria2://`、`hy2://` 或 `trojan://` 分享链接，先显示不含凭据的审查结果再写入待提交 UCI；无法由当前节点模型保留的传输或安全参数会明确拒绝，不会静默丢弃；
- 首批逻辑出口只支持单节点、直连和阻断；连接失败后的自动切换尚未实现。
- 混合规则的 DNS 投影只使用查询阶段可见字段，Route 投影保留目标 IP、端口和 TCP/UDP；只有连接阶段条件时不会生成空 DNS 规则。
- 源 MAC 是普通客户端条件：字段内多个 MAC 为 OR，并与其他非空字段 AND。当前 sing-box 1.13 基线由 nftables `ether saddr` 分类到专用双栈入口，不做 MAC→IP 解析；UCI 使用 1.14 原生字段名 `source_mac_address`，以后切换原生后端不迁移规则。
- GeoSite/GeoIP 由独立的版本锁定数据包提供，并转换为受控本地 sing-box 规则集；数据源不能携带 DNS、出口或 fallback 动作。

更完整的设计、审计证据与开发边界见 [项目规划](docs/PROJECT_PLAN.md)；跨工具生命周期对照、
生产阻塞项和“仅可停用安装”的边界见 [生产就绪审计](docs/PRODUCTION_READINESS.md)。

## 仓库结构

- `steer-openwrt/`：Go 意图编译器、OpenWrt adapter、UCI 与 procd 包；
- `steer-geodata/`、`geoview/`：固定 Geo 数据和独立转换工具包；
- `luci-app-steer/`：LuCI 应用、独立的简体中文语言包与最小权限 RPC；
- `tests/`：语义模型、回归与目标机验证；
- `docs/`：规划、上游选择与公开的工程验证记录。

## 当前状态

当前后端已经建立 schema 4 Canonical Intent、严格引用校验、确定性编译、sing-box/nft 原生
预检和可运行的 OpenWrt 接管闭环。procd 只监督发行版 sing-box；普通流量使用 TUN
`auto_route`/`auto_redirect`，Steer 只保留传统 DNS redirect 与 sing-box 1.13 源 MAC 能力所需
的最小 nft shim。GeoSite/GeoIP 由包管理器拥有的固定数据生成本地 `.srs`。

LuCI 已覆盖总览、普通规则、具名本地代理、DNS Profile、节点和逻辑出口。LuCI“保存并应用”
提交 UCI 后由 OpenWrt `config.change` 自动触发一次 Apply，前端只等待该结果，不会再发第二次
Apply。单节点分享 URL 在浏览器本地严格解析；这不等于订阅更新。

当前后端提供以下主要命令：

- `steer validate`：输出结构化错误和警告；
- `steer compile` / `compile-sing-box` / `compile-firewall`：输出编译产物；
- `steer plan` / `status` / `health`：查看计划、候选与运行状态；
- `steer apply`：串行预检、切换并验证本地运行态；
- `steer probe`：手动执行当前 Plan 的 HTTPS 诊断，不影响 Apply；
- `steer rollback`：一次性恢复上一份本地健康 UCI，并复用正常 Apply。

LuCI/ubus commit 会自动触发 reload。终端裸 `/sbin/uci commit` 不发送 ubus 事件，因此 CLI
修改使用 `uci commit steer && /etc/init.d/steer reload`。Steer 不包装系统 `uci`，也不运行
常驻文件监视器。

## 许可证

Steer 以 GPL-3.0-or-later 发布，许可证全文见 [LICENSE](LICENSE)。第一版运行集成参考
OpenWrt-momo 的工程结构，但 Steer 使用自己的语义模型和实现；其他上游只作为行为与工程
参考。SDK、geoview、GeoSite/GeoIP 等直接发布输入及固定哈希见
[发布输入与来源](docs/SOURCES.md)。
