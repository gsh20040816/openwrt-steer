# 开发与验证环境

## 基线

日常集成验证使用 KVM 虚拟机，不在生产路由器上反复试错：

- 官方 OpenWrt 25.12.5 x86/64 ext4 镜像；
- Linux 6.12.94；
- SmartDNS 46.1；
- sing-box 1.13.14，与当前目标路由器一致；
- 临时 qcow2 overlay，测试结束后可以直接丢弃。

OpenWrt 25.12.5 官方软件源当前提供 sing-box 1.12.17。它可用于下限兼容检查，但不能代替目标 1.13.14 验证。测试使用的 1.13.14 二进制必须先核对来源与 SHA-256，不能运行复制不完整的文件。

## 集成测试

将仓库复制到一次性 OpenWrt x86/64 VM 后运行：

```sh
SING_BOX_BIN=/usr/bin/sing-box-1.13.14 \
  tests/integration/run-openwrt-vm.sh
```

脚本会：

1. 运行 ucode 语义模型测试；
2. 安装当前工作树中的编译器、运行层和 init 脚本测试副本；
3. 用完全虚构的代表性 fixture 编译 sing-box 与两个 SmartDNS 实例；
4. 验证 SmartDNS 业务上游不生成内部 SOCKS，防回环 mark 只跳过本机 53 劫持且仍进入 output TPROXY 和普通 Rules；
5. 执行 sing-box 原生 `check`；
6. 分别启动 SmartDNS 实例并检查进程能保持运行；
7. 通过 procd 启动 sing-box 与多个 SmartDNS 实例；
8. 检查双栈 TPROXY 策略路由、按 `fw4 zone` 实际设备生成的 nftables 规则、源 MAC 专用双栈入口和 firewall4 reload 后恢复；
9. 实际发起路由器本机 UDP/123 请求，确认系统 NTP 在普通规则标记前固定直连并由 RPC 报告计数；
10. 确认 mark 冲突在切换前被拒绝且当前运行代保持健康；
11. 确认通用悬空引用 fixture 必须失败；
12. 停止服务并验证 nftables 表被撤销，随后恢复测试前的 Steer 与 firewall 配置。

该脚本会短暂写入 VM 的 `/usr/share/steer`、`/usr/libexec/steer`、`/usr/sbin/steerctl`、`/etc/init.d/steer`、`/etc/steer` 和 `/var/lib/steer`，并重载 VM 的 firewall4，因此只能在一次性测试 VM 中运行，不能直接在生产路由器执行。

当前集成测试验证的是控制面启动、规则装载、源 MAC 外部编译结果和切换前拒绝。当前 KVM
镜像没有 veth 模块，尚不能在单 VM 内构造独立 LAN 客户端；真实客户端 TCP、普通 UDP、QUIC、
源 MAC 的 IPv4/IPv6 数据包、IPv4/IPv6 GUA、flow offload 隔离和切换后故障回滚仍需在带独立
LAN 网卡/客户端 VM 的拓扑中补齐。未完成前不得据此批准生产切换。

本机构建耗时只用于同一主机、同一 SDK 镜像下的优化前后 A/B，不能外推为 GitHub Actions
耗时。CI 性能结论必须来自 GitHub 托管 runner 的实际工作流记录，并区分首次缓存未命中与后续
缓存命中。

## LuCI 验证

LuCI 第一版在同一台 OpenWrt KVM 中安装测试副本，并实际验证：

- `luci.steer` 的 `status`、`validate`、`apply` RPC 注册和返回值；
- 通过 RPC 首次启动后 core、DNS Profile、网络接管和 last-known-good 状态；
- Overview、Rules、Local Proxies、DNS 与 Nodes & Routes 页面无新 JavaScript 运行错误；
- 规则页只显示决策列，协议细节留在 modal；
- 普通规则的域名和目的 IP 使用多行 IDE 式编辑器；光标所在行输入 `geosite:` 或 `geoip:` 后实时过滤当前数据中的合法名称，并支持键盘或鼠标补全；
- 验证域名字段内各表达式和目的 IP 字段内各表达式为 OR，所有非空字段之间由 sing-box 显式 logical AND 连接；
- Default 固定在普通规则末尾，没有编辑、删除、启停或拖动入口；
- 390 px 窄屏下意图路径转为纵向，表格保持 LuCI 原生横向滚动。

浏览器验证只在一次性 VM 上执行，没有通过界面改写生产路由器。`tests/node/luci_view_test.js`
提供空候选项、悬空引用与可选面板的快速页面模块回归，但使用的是最小 LuCI 表单桩，不能替代
上述 OpenWrt KVM 真实浏览器验证。

## 目标设备边界

目标路由器只用于：

- 读取不含配置秘密的二进制版本信息；
- 第一版在 VM 完成接管、回滚和故障测试后，执行经用户确认的最终验收。

在此之前不得启用 Steer、改写生产 UCI、重载 firewall4，或启停生产代理与 DNS 服务。私人配置、流量记录与迁移对照不得写入公开仓库或测试 fixture。
