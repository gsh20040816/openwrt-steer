# 架构

Steer 采用“共享意图核心 + 平台适配器”。共享编译器生成可直接交给 sing-box 的最终配置；平台适配器提供该平台所需的入站片段，并单独规划和应用操作系统资源。不存在公开 Execution Plan，也不存在 UCI 到其他平台配置的兼容桥。

## 代码边界

```text
go/internal/intent                 Canonical Intent、schema 7 校验、严格 JSON codec
go/internal/compiler               确定性 sing-box 最终配置编译
go/internal/apply                  同步 Apply 编排和结果合同
go/internal/generation             平台中立 generation 文件
go/internal/subscription           抓取、解析、合并与窄 Store 接口
go/internal/probe                  HTTP/TLS 测量和报告
go/internal/capability             sing-box 版本/build-tag 能力检查
go/internal/platform/openwrt       OpenWrt 网络、服务、UCI、Geo 和日志适配
go/internal/platform/openwrt/uci   严格 UCI 语法解析
go/cmd/steer-openwrt               OpenWrt CLI
```

共享包只能依赖共享包；`platform/openwrt` 可以依赖共享包，反向依赖禁止。

## 数据流

```text
OpenWrt UCI ──严格解码──┐
未来 JSON ──严格解码───┼→ Canonical Intent → Validate → Compiler
                       │                         │
                       └─────────────────────────┘
                                                   ↓
                                      final sing-box config
                                                   +
                                      platform-native plan
                                                   ↓
                                   Prepare / Activate / Healthy / Finalize
```

编译结果由 Intent、明确状态目录和平台提供的 sing-box Target 决定。时间、探测结果、订阅抓取状态和操作系统运行信息不会混入 Intent 或编译结果。

## 路由与 DNS

`route.detour` 是 single Route 到另一条 single Route 的有向边：

```text
应用流量 → exit route/node → front route/node → 网络
```

校验器拒绝悬空、禁用、非 single 目标和任意环。编译器为每条 single Route 生成独立协议出站 `steer-route-<id>`，因此同一节点可以在不同 Route 中拥有不同前置链，不需要全局节点 selector。

每个启用规则实际引用的 `(DNS Profile, Route)` 编译成独立 DNS transport。代理 Route 引用同一个 Route tag，自动继承完整前置链；Direct 不写 detour；Block 投影为 DNS reject。普通规则与 DNS 规则共享可表达的匹配条件，目标 IP、网络、协议和端口只参与业务流量。

## OpenWrt 数据面

主路径由 sing-box TUN 的 `auto_route`、`strict_route` 和 `auto_redirect` 接管。平台适配器补充两项 1.13 基线能力：

- nftables 把传统 TCP/UDP 53 送入专用 sing-box DNS 入口；
- 源 MAC 条件使用专用双栈 TProxy/DNS 入口和策略路由。

TUN 名称、地址、table、priority、mark、NFQUEUE 和平台端口都属于 `platform/openwrt`，不出现在 Canonical Intent。特殊非全球地址在进入用户规则前排除，路由器本机 UDP/123 明确直连。

## Apply

公共 Apply 是单锁、同步流程：

1. 平台 codec 严格解码配置；
2. 共享校验器拒绝非法 Intent；
3. 共享编译器生成最终 sing-box 配置；
4. `Prepare` 检查 sing-box 能力、准备 Geo、生成候选并执行 `sing-box check` 与 `nft -c`；
5. `Activate` 停止旧 procd 实例、应用平台资源、发布 `current`，再启动候选；
6. `Healthy` 检查 procd、TUN、nftables 和必要监听端口；
7. `Finalize` 删除其他运行 generation。

候选预检查失败发生在运行态变更前。`current` 在候选进程启动前发布，供 procd 读取。切换后的失败返回错误并保留现场；没有自动恢复、旧配置备份或 rollback 接口。禁用走单独的 `Disable`，停止服务并删除运行资源。

开机时 procd 通过非公开 `_start` 钩子执行校验、编译、Prepare 和平台激活，然后由同一个 init 脚本创建 sing-box 实例。该钩子不是公共 CLI，也不构成第二套配置语义。

## Generation 与状态

```text
/run/steer/generations/<id>/intent.json      Canonical Intent
/run/steer/generations/<id>/sing-box.json    最终核心配置
/run/steer/generations/<id>/platform.json    OpenWrt 内部资源计划
/run/steer/generations/<id>/firewall.nft     OpenWrt nftables 辅助层
/run/steer/current                            当前 generation 链接
/run/steer/last-apply.json                    最近一次公共 Apply 结果
```

公共 `status` 只返回：

```json
{"healthy": true, "last_apply": {"sequence": "…", "result": {"ok": true}}}
```

配置诊断由 `validate` 独立返回。组件细节留在平台健康检查和系统日志中，不扩张跨平台状态合同。

## 订阅与测试

共享订阅逻辑只依赖窄 `Store`：替换一组订阅节点或删除一个节点。OpenWrt Store 生成单次 UCI batch；未来 JSON Store 可以原子写配置文件。订阅只改变节点，提交后不主动 Apply。

共享 probe 负责 HTTP/TLS 测量。OpenWrt 概览测试读取当前 generation 的 `intent.json`，节点和路由测试读取磁盘 UCI并临时启动环回 sing-box。测试报告只写 `/var/lib/steer/logs/tests`，不会进入配置或编译输入。
