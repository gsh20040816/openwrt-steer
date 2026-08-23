# 开发与验证

0.5 的任务是固定统一项目命名、跨平台边界和发布契约，同时保持 OpenWrt 正常使用可靠。功能边界已经冻结；修改应落在正确包中，不通过兼容桥保留旧目录、旧 CLI 或旧状态结构。

## 仓库结构

```text
go/internal/{intent,compiler,apply,generation}  共享语义和生命周期
go/internal/{subscription,probe,capability}     共享服务
go/internal/platform/openwrt                    OpenWrt 适配器
go/internal/platform/linux                      Linux systemd 主机/转发流量适配器
go/cmd/steer-openwrt                            OpenWrt CLI 源码 target
go/cmd/steer-linux                              Linux CLI/Web
linux                                           Linux 通用发行资产与 systemd unit
luci-app-steer                                  LuCI、RPC、ACL、翻译
steer                                           OpenWrt 控制器包
steer-geodata                                   固定 Geo 数据包
geoview                                         上游工具原样打包
tests/node                                      LuCI/分享 URL 回归
tests/integration                               一次性 OpenWrt VM 正常路径
```

共享模块路径为 `github.com/gsh20040816/steer/go`，OpenWrt 控制器包和命令源码均使用统一名称 `steer`。

## 本地验证

```sh
cd go
gofmt -w <changed-go-files>
go test ./...

cd ..
node tests/node/share_url_test.js
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
python3 tests/check-linux-packaging.py
git diff --check
```

修改后必须运行相关测试；提交和发布前运行完整集合。不能删除、跳过或弱化失败测试。当前不建设故障注入矩阵；验证集中于用户正常配置、正常 Apply、启动、订阅和诊断路径，以及正常输入中的 fail-fast 边界。

## 包职责

- `intent` 只定义用户语义。新增字段必须有跨平台意义，并同时进入严格 JSON/UCI codec 和校验。
- `compiler` 输出最终 sing-box 配置与能力/Geo 需求，不输出 nftables、服务管理器或公共平台计划。
- `apply` 保持同步 KISS，仅编排五个 Backend 方法，不加入重试、回滚、事件总线或工作流引擎。
- `generation` 只拥有 `intent.json` 和 `sing-box.json`；平台文件由平台适配器添加。
- `subscription.Store` 必须保持窄接口，不能让共享逻辑依赖 UCI 命令。
- `probe` 只测量和报告；目标选择、临时核心进程与日志路径由平台适配器处理。
- `platform/openwrt` 是 UCI、nftables、策略路由、procd 和 OpenWrt 目录的唯一所有者；显式 Geo 数据文件到内容寻址 SRS generation 由 `internal/geodata` 共享。

## 公共契约

公开 CLI 只有：

```text
version validate apply health status probe subscription geo-catalog cleanup
```

`_start` 仅供 init 脚本使用，不属于公共接口。不得重新公开 compile、plan、prepare、capabilities 或 rollback。RPC 只包装用户/界面需要的公共操作；状态对象固定为 `healthy + last_apply`，合法性由 validate 单独返回。

## Linux 交付边界

Linux 适配器与上游发行资产已经建立，后续修改必须保持：

1. 源码 target 继续叫 `cmd/steer-linux`，安装名固定为 `/usr/bin/steer`；
2. 主 CI 只构建 x86_64/aarch64 通用 tar.zst，不构建 deb、rpm、pkg.tar 等发行版包；
3. Linux 包不依赖特定 Geo 数据包，只消费用户在 `platform.json` 中选择的 `.dat`；
4. sing-box、geoview、nftables、iproute2 和 ca-certificates 始终由系统包管理器提供；
5. tag 发布只复用同一 master commit 已验证的 OpenWrt 与 Linux 产物，不重新编译。

macOS 在 Linux 接口稳定后开始，允许使用 launchd、utun/pf 和不同权限模型，但不得改变共享规则、路由、DNS 或订阅语义。

## OpenWrt VM 与发布门

`tests/integration/run-openwrt-vm.sh` 只能运行在一次性 OpenWrt 25.12.5 x86/64 VM。它会替换 UCI、运行目录、nftables、策略路由和 procd，并覆盖公开 RPC、正常 Apply/reload/restart、DNS/MAC 辅助层、禁用/重新启用及非法配置 fail-fast。

发布门：

1. 全部本地检查通过；
2. 官方 OpenWrt SDK 完整构建五个 APK，Linux 并行构建两个通用 tar.zst；
3. release bundle 中全部产物来自拟发布 commit；
4. 安装后运行 `validate`、`health`、`status` 和显式测试；
5. 预发布版本在真实目标上满足稳定门槛后再晋级稳定版。
