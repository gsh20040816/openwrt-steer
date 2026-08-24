# 范围与冻结规则

本文是 0.4 跨平台迁移的范围基线。功能分为“共享语义”和“平台实现”两层；判断一个功能是否应进入共享核心时，只看它是否改变用户意图，而不看当前 OpenWrt 恰好怎样实现。

## 第一层：共享语义

以下内容已经实现并在 0.4 alpha 期间冻结：

- schema 8 Canonical Intent：主配置、Bootstrap、节点、订阅、逻辑路由、DNS Profile、本地代理和规则；
- 严格解码：UCI 适配器拒绝未知 section/option 和 scalar/list 形态错误；Canonical JSON codec 拒绝未知字段与尾随数据；
- 全局 ID、启用引用、协议参数、端口、URL、唯一 Direct、唯一 Default 等语义校验；
- first-match 规则，同字段 OR、不同字段 AND，Default 决定最终 DNS 和业务路由；
- 单节点路由前置链：目标必须存在、启用且同为 single，悬空、禁用、类型错误、自环和间接环全部拒绝；
- 确定性 sing-box 最终配置编译，包括 Route 私有出站、DNS 路径、Geo 引用和能力需求；
- 同步 Apply 生命周期 `Prepare → Activate → Healthy → Finalize` 与 `Disable`；
- HTTP(S) 订阅的抓取、解析、合并、稳定 ID、stale pin 和窄持久化接口；
- HTTP/TLS 测量、连接测试和完整下载报告格式。

共享核心不认识 UCI、procd、launchd、systemd、nftables、pf、路由表号或平台目录。`status` 的公共语义只有 `healthy` 和 `last_apply`；配置合法性由独立 `validate` 返回。

## 第二层：平台实现

OpenWrt 适配器当前拥有：

- UCI schema 8 codec、显式 schema 7→8 窄迁移和 UCI 订阅节点持久化；
- sing-box TUN `auto_route`/`auto_redirect` 入站；
- 传统 TCP/UDP 53 DNS 捕获和源 MAC TProxy 辅助层；
- nftables、双栈策略路由、固定 mark/table/priority/NFQUEUE 资源；
- procd 生命周期、开机私有 `_start` 钩子、本地健康检查；
- 包内完整 SRS seed、manifest 精确 selector 校验与 sing-box remote rule-set；
- `/run/steer` generation、`/var/lib/steer` 日志和订阅状态；
- LuCI、ucode RPC、ACL、OpenWrt 包和 cron 订阅调度。

这些是 OpenWrt 实现，不得反向污染共享 Intent。Linux 当前使用 JSON 配置与 systemd；macOS 仍可使用 JSON 配置与 launchd/pf。只要共享语义和 Apply 生命周期一致，平台目录、网络接管方式、权限模型和服务管理器可以不同。

## 当前不做

- macOS 的实际网络接管、安装包和 UI；Linux 的发行版安装包仍不在范围内，但 systemd/loopback Web 源码适配器已实现；
- 自动/人工回滚、配置历史、启动时选择旧 generation；
- 节点故障转移、健康调度、隐藏 fallback；
- 资源所有权协商或与第三方代理栈共存；
- 故障注入测试矩阵；
- DIRECT 内核旁路、flowtable、运行时 outbound 命中解释；
- 新协议、新规则条件、订阅策略扩展或 LuCI 功能扩张。

配置或运行环境不满足前提时应 fail-fast。只有正常使用中能够触发并影响正确性的缺陷、安全问题或被支持依赖的必要变更，可以打破冻结。

## 冻结与晋级

- `v0.4.0-alpha.1` 起功能边界硬冻结；共享接口进入候选冻结。
- `v0.4.0` 起进入稳定版本线；`0.4.x` 只接受正常使用缺陷、安全问题和必要依赖适配，不增加公共功能。
- 共享核心与 OpenWrt 功能边界保持冻结；后续跨平台实现从 Linux 适配器开始，macOS 排在 Linux 之后。
