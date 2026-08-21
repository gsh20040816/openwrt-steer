# 开发与验证环境

M1 重构边界见 [重构冻结基线](REFACTOR_BASELINE.md)，包所有权和更新规则见
[打包与文件所有权](PACKAGING.md)。GeoSite/GeoIP 版本只通过 `steer-geodata` 包更新；集成测试
不再启动联网 updater 或调度服务。

## 基线

日常集成验证使用 KVM 虚拟机，不在生产路由器上反复试错：

- 官方 OpenWrt 25.12.5 x86/64 ext4 镜像；
- Linux 6.12.94；
- sing-box 1.13.18（`with_quic`、`with_utls`）；
- 临时 qcow2 overlay，测试结束后可以直接丢弃。

测试使用的 sing-box 二进制必须先核对来源、版本、build tags 与 SHA-256，不能运行复制不完整
或能力不足的文件。

## 集成测试

将仓库复制到一次性 OpenWrt x86/64 VM 后运行：

```sh
SING_BOX_BIN=/usr/bin/sing-box \
  tests/integration/run-openwrt-vm.sh
```

脚本会：

1. 安装当前工作树的 `steer`、init 和 LuCI RPC 测试副本；
2. 用完全虚构的 schema 6 fixture 编译多种 sing-box 1.13 节点、六种 DNS transport、Geo 与本地代理；
3. 执行 sing-box 和 nftables 原生检查；
4. 通过 procd 启动 sing-box，检查 TUN、NFQUEUE、双栈 DNS/MAC shim 和实际 DNS 查询；
5. 用真实 authenticated UCI session 复现 LuCI 生命周期，确认首次 commit 恰好触发一次 Apply；
6. 检查第二次 commit、`/etc/init.d/steer reload`、fw4 reload 和 procd respawn；
7. 确认 HTTPS probe 只在 `steer probe` 手动执行，不阻断 Apply；
8. 通过 `luci.steer rollback` 恢复并消费唯一健康 UCI 备份，防止 rpcd 自调用死锁；
9. 确认非法 schema 在切换前失败，禁用后下一次 commit 仍能自动启用；
10. 清理 nftables、策略路由、测试桥和临时文件，恢复测试前配置。

该脚本会短暂写入 VM 的 `/usr/sbin/steer`、`/etc/init.d/steer`、rpcd ucode、
`/run/steer` 和 `/var/lib/steer/rollback.uci`，并重载 firewall4，因此只能在一次性测试 VM 中
运行，不能直接在生产路由器执行。

当前集成测试验证的是控制面启动、规则装载、源 MAC 外部编译结果和切换前拒绝。当前 KVM
镜像没有 veth 模块，尚不能在单 VM 内构造独立 LAN 客户端；真实客户端 TCP、普通 UDP、QUIC、
源 MAC 的 IPv4/IPv6 数据包、IPv4/IPv6 GUA、flow offload 隔离和切换后故障恢复仍需在带独立
LAN 网卡/客户端 VM 的拓扑中补齐。未完成前不得据此批准生产切换。

本机构建耗时只用于同一主机、同一 SDK 镜像下的优化前后 A/B，不能外推为 GitHub Actions
耗时。CI 性能结论必须来自 GitHub 托管 runner 的实际工作流记录，并区分首次缓存未命中与后续
缓存命中。

## LuCI 验证

LuCI 第一版在同一台 OpenWrt KVM 中安装测试副本，并实际验证：

- `luci.steer` 的 `status`、`validate`、`plan`、`rollback` RPC 注册和返回值；
- 会话化 UCI commit 后 core、DNS、网络接管、last_apply 与 rollback 状态；
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
