# 开发与验证

0.5 alpha 的任务是验证统一项目命名和跨平台边界，同时保持 OpenWrt 正常使用可靠。功能边界已经冻结；修改应落在正确包中，不通过兼容桥保留旧目录、旧 CLI 或旧状态结构。

## 仓库结构

```text
go/internal/{intent,compiler,apply,generation}  共享语义和生命周期
go/internal/{subscription,probe,capability}     共享服务
go/internal/platform/openwrt                    OpenWrt 适配器
go/internal/platform/linux                      Linux systemd 主机/转发流量适配器
go/cmd/steer                                    OpenWrt CLI
go/cmd/steer-linux                              Linux CLI/Web
linux/systemd                                   Linux unit 文件（手工安装参考）
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

## 新平台开发顺序

稳定版晋级后，Linux 适配器按以下顺序开始：

1. 实现 schema 7 严格 Canonical JSON 文件读写和 revision/ETag；
2. 实现平台目录、权限和 systemd 生命周期；
3. 选择并验证 Linux 网络接管方式，生成平台内部计划；
4. 接入共享 Backend 五方法；
5. 复用 subscription/probe，并实现 JSON Store 和平台日志路径；
6. 提供 loopback Web API/UI，与 CLI 共享同一套 Apply；
7. 暂不维护 GitHub Actions Linux 打包或发行版打包脚本。

macOS 在 Linux 接口稳定后开始，允许使用 launchd、utun/pf 和不同权限模型，但不得改变共享规则、路由、DNS 或订阅语义。

## OpenWrt VM 与发布门

`tests/integration/run-openwrt-vm.sh` 只能运行在一次性 OpenWrt 25.12.5 x86/64 VM。它会替换 UCI、运行目录、nftables、策略路由和 procd，并覆盖公开 RPC、正常 Apply/reload/restart、DNS/MAC 辅助层、禁用/重新启用及非法配置 fail-fast。

发布门：

1. 全部本地检查通过；
2. 官方 OpenWrt SDK 完整构建并产出五个 APK；
3. 构建产物来自拟发布 commit；
4. 安装后运行 `validate`、`health`、`status` 和显式测试；
5. alpha 在真实路由器正常使用至少一周后再晋级稳定版。
