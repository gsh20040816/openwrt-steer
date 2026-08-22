# 开发与验证

Steer 主线面向 OpenWrt 25.12.5 x86/64 和 sing-box 1.13.18。修改必须先证明 Canonical Intent 与编译不变量，再验证 OpenWrt adapter 和 LuCI；不能用浏览器校验代替后端校验，也不能只靠 JSON 形状测试冒充 sing-box 原生可用。

## 仓库结构

```text
steer-openwrt/src/internal/model       Canonical Intent 与语义校验
steer-openwrt/src/internal/compiler    Execution Plan 与 sing-box 编译
steer-openwrt/src/internal/openwrt     UCI、Apply、平台资源、订阅和测试
steer-openwrt/src/cmd                  /usr/sbin/steer CLI
luci-app-steer                         LuCI 页面、RPC、ACL、翻译
steer-geodata                          固定 Geo 数据包
geoview                                上游工具打包与补丁
tests/node                             LuCI 与分享 URL 回归
tests/integration                      一次性 OpenWrt VM 集成
```

## 本地测试

Go 模块位于 `steer-openwrt/src`：

```sh
cd steer-openwrt/src
gofmt -w <changed-go-files>
go test ./...
```

仓库根目录的前端和边界检查：

```sh
node tests/node/share_url_test.js
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
git diff --check
```

每次修改必须运行与改动直接相关的测试；准备提交和发布时运行以上全部检查。不得删除、跳过或弱化失败测试。

## 代理链回归要求

`route.detour` 修改至少覆盖：

- 单级和多级合法链；
- 同一节点由多条路由以不同 detour 复用；
- 自环、两节点环和更长间接环，错误包含完整路径；
- 悬空、禁用、Direct、Block 目标；
- 生成的每条单节点 Route 都是协议出站，不残留全局 node selector；
- DNS transport 引用 Route tag 后包含同一代理链；
- 完整生成配置通过目标 sing-box 1.13 的 `check`。

在线 sing-box 文档可能已经描述 1.14 或更新行为，不能据此放宽 1.13 输出。测试二进制必须先核对版本和 build tags。

## 诊断回归要求

测试功能至少覆盖：

- 三个必填 URL 只能使用 UCI scalar option；
- 连接成功、HTTP 失败、失败后第二次成功、完整下载和超时；
- 概览从当前 Plan 选择正确 URL；
- 节点连接使用 `probe_proxy`，节点下载使用 `speedtest_proxy`；
- 路由测试的 final 为目标 Route tag，且包含全部 detour 出站；
- 禁用节点/路由被拒绝；
- 报告权限为 `0600`，目录按 overview/nodes/routes 隔离且覆盖最新同类报告；
- RPC 参数声明、shell quoting、ACL 和 LuCI 非持久按钮；
- 概览没有 rollback 操作。

## schema 版本管理

公开 schema 变化必须同时修改 model 常量、UCI scalar/list 形态、默认配置、fixture、LuCI、包安装脚本、边界测试和文档。

0.3.0 正式版只接受 schema 7，不携带 alpha 阶段的 schema 6 转换和旧运行计划兼容代码。发布验证必须确认新装默认配置和待升级设备均已是 schema 7；版本不匹配时必须明确失败且不能改写 UCI。

## OpenWrt VM

`tests/integration/run-openwrt-vm.sh` 只允许在一次性测试 VM 运行。它会替换 `/etc/config/steer`、加载 nftables/策略路由、启动 procd、写入 `/run/steer` 和 `/var/lib/steer`，并执行显式 rollback 与清理。

典型入口：

```sh
OPENWRT_HOST=<test-vm> \
SING_BOX_BIN=/usr/bin/sing-box \
tests/integration/run-openwrt-vm.sh
```

当前 VM 主要验证控制面、配置编译、原生检查、procd、TUN、DNS/MAC 辅助规则、RPC 注册和 Apply 事务。没有独立 veth LAN 客户端时，不能声称已经覆盖真实 LAN IPv4/IPv6 TCP、UDP、QUIC、GUA、flow offload 或跨接口二层行为。

## 真机发布门

正式版发布前至少确认：

- 目标 OpenWrt、sing-box 版本和 build tags；
- UCI schema 7 内容与 `/etc/rc.d` 状态；
- `steer validate`、`plan`、`apply`、`health`、`status`；
- procd 进程、TUN、nftables、监听端口；
- 概览 direct/proxy/speedtest；
- 一个裸节点连接与下载测试；
- 一条路由链连接与下载测试；
- `/var/lib/steer/logs/tests` 的路径、内容与权限；
- 重载 LuCI 后没有未知 UCI 字段。

如果生产配置没有可用多级链，不得为了验收改变真实路由关系。多级 detour 应在隔离 fixture 或 VM 中验证，真机只测试已有合法路径。

## 发布流程

1. 完整测试通过并确认工作树只包含本次变更；
2. 使用英文 `<type>: <message>` 提交到 `master`；
3. 推送后等待 `Build OpenWrt packages` 对同一 commit 成功；
4. 校验构建资产的 `SHA256SUMS` 和 `BUILD-METADATA.txt`；
5. 在该 commit 创建版本 tag；
6. `Publish tagged release` 只能复用同一 commit 的成功 master 构建，发布为 GitHub prerelease；
7. 从 Release 资产安装真机，而不是使用本地未发布包；
8. 记录实际版本和验收结果。
