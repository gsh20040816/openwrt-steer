# 架构

Steer 将平台中立的网络意图与 OpenWrt 执行机制分开。UCI adapter 负责解码公开配置，Canonical Intent 负责语义，编译器生成确定性 Execution Plan 和 sing-box JSON，OpenWrt adapter 才处理 procd、nftables、策略路由与运行目录。

## 数据流

```text
/etc/config/steer
        │ 严格 UCI 解码：拒绝未知 section、option 和错误 scalar/list 形态
        ▼
Canonical Intent (schema 7)
        │ 引用、类型、能力、唯一性、代理链环检测
        ▼
Validation + deterministic compiler
        ├── Execution Plan
        ├── sing-box.json
        ├── firewall.nft
        └── semantic object digests
        │ sing-box check + nft -c
        ▼
/run/steer/generations/<id>
        │ Apply 串行切换
        ▼
/run/steer/current → generation
        │
        ├── procd → distribution sing-box
        └── OpenWrt nftables / policy routing resources
```

编译输出只由 Intent 和明确的状态目录决定。相同输入必须生成相同 Plan、对象摘要和 sing-box 结构；运行时间、探测结果与订阅抓取状态不会混入 Intent。

## Canonical Intent

schema 7 包含八类对象：

- `steer`：启用状态、日志、三个必填测试 URL、DNS cache 参数；
- `bootstrap`：启动 DNS 根；
- `node`：协议、认证、传输、TLS/REALITY 和订阅来源；
- `subscription`：不可信节点数据源；
- `route`：Direct、Block 或单节点逻辑路由，以及可选 `detour`；
- `dns_profile`：业务 DNS transport；
- `local_proxy`：显式本地 SOCKS/Mixed 入口；
- `rule`：有序匹配条件和 DNS/Route 动作。

所有公开对象 ID 在全局命名空间唯一。启用引用必须存在且目标启用；禁用对象可以作为未完成草稿保留，但不能被启用对象引用。

## 前置代理图

`route.detour` 定义单节点路由之间的有向边：

```text
业务连接 → exit route/node → front route/node → 物理网络
```

合法图必须满足：

1. 边的起点与终点都是启用的 `single` 路由；
2. 目标存在且启用；
3. 图中不存在自环或间接环；
4. Direct 和 Block 不携带也不充当前置路由。

校验器按配置顺序执行深度优先遍历并维护访问栈。发现回边时返回包含首尾重复节点的完整路径，例如 `a -> b -> c -> a`。LuCI 不实现第二套环检测，避免浏览器暂存状态和后端语义分叉。

前置关系属于 Route，不属于 Node。同一节点被两条路由引用时，编译器复制该节点的强类型出站参数，为每条路由使用 `steer-route-<id>` tag，并只在对应副本写入 `detour: steer-route-<upstream-id>`。完整运行配置不再生成全局 `steer-node-*` selector；裸节点测试才临时使用节点 tag。

## DNS 与规则投影

编译器为每个启用规则实际引用的 `(DNS Profile, Route)` 组合生成独立 DNS transport。Direct 路径没有 detour；单节点路径引用逻辑 Route tag，因此自动包含该 Route 的完整前置链；Block 在 DNS 投影中变为 reject。

普通规则分别投影到 sing-box route 和 DNS 规则：

- 域名、源地址和入口等共同条件进入两边；
- 目标 IP、网络、协议和端口只进入普通流量路由；
- 同字段多值为 OR，不同字段用显式 logical AND；
- Default 决定 sing-box route final 和 DNS final。

GeoSite/GeoIP 数据由 `steer-geodata` 提供固定输入，`geoview` 提取被引用分类后再由配套 sing-box 编译为本地 SRS。远程数据只能提供匹配集合，不能携带动作。

## OpenWrt 数据面

主路径使用 sing-box TUN：

- `auto_route`、`strict_route`、`auto_redirect`；
- 固定 TUN 名、地址、路由表、优先级、mark 和 NFQUEUE；
- 非全局特殊地址排除；
- 路由器本机与 LAN 客户端进入同一规则体系。

Steer 的 nftables 辅助层只承担两类 1.13 基线缺口：

- 把传统 TCP/UDP 53 重定向到 sing-box DNS 入口；
- 按二层源 MAC 把 TCP/UDP 流量送入专用双栈 TProxy/DNS 入口。

Bootstrap DNS 与必要的系统启动流量不依赖用户代理链。防回环使用编译器固定且互不冲突的 mark、表号和优先级。

## Apply 与运行状态

Apply 顺序为：

1. 严格读取当前 UCI；
2. 校验并生成候选 Plan；
3. 运行 sing-box 和 nftables 原生离线检查；
4. 如果当前运行代本地健康，保存单份 `rollback.uci`；
5. 停止旧实例、激活候选资源、通过 procd 启动 sing-box；
6. 检查进程、TUN、nftables 和监听端口；
7. 成功后只保留当前 generation。

候选在切换前失败不会影响当前运行代。切换后的失败保留现场，不自动恢复；重启时也不会自动选择 rollback。`steer rollback` 是一次性人工入口，LuCI 概览不再展示它。

## 诊断模型

诊断分为三层，但共享同一 HTTP 测量和报告格式：

- `overview`：读取当前 Plan，直接从路由器发起请求，让当前运行规则决定路径；
- `nodes/<id>`：读取磁盘 UCI，临时生成单个裸节点出站；
- `routes/<id>`：读取磁盘 UCI，只编译目标 Route 及其完整 detour 祖先，以目标 Route tag 作为 final，避免无关路由污染测试。

连接测试记录 TCP、TLS、TTFB、HTTP 状态和尝试次数；完整下载记录字节与耗时。报告只进入 `/var/lib/steer/logs/tests`，不进入 UCI、Plan 或对象摘要。

## 目录与所有权

```text
/etc/config/steer                         用户配置
/usr/sbin/steer                           控制 CLI
/etc/init.d/steer                         procd 服务
/usr/share/steer/geodata-seed             包拥有的 Geo 输入
/var/lib/steer/geodata                    生成的本地规则集
/var/lib/steer/subscriptions              订阅快照
/var/lib/steer/logs/tests                 最新诊断报告
/var/lib/steer/rollback.uci               单份人工恢复点
/run/steer/generations                    临时候选与当前代
/run/steer/current                        当前 generation 符号链接
/run/steer/last-apply.json                最近 Apply 结果
```

`/run/steer` 可在重启后重建；`/var/lib/steer` 是持久生成状态，但不取代 `/etc/config/steer` 的配置权威地位。
