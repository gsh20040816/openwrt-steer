# 测试说明

Steer 的测试按语义、编译、OpenWrt 适配、LuCI、打包和真实运行环境分层。任何一层通过都不能替代其他层；发布结论必须准确说明实际运行了哪些验证。

## Go 单元与回归测试

```sh
cd steer-openwrt/src
go test ./...
```

主要覆盖：

- schema 7 UCI scalar/list 形态和未知字段拒绝；
- Canonical Intent 引用、Route detour DAG、完整环路径和非法目标；
- route-private 协议出站、detour tag、DNS transport 与规则编译；
- Apply 候选、当前 Plan、状态和 cleanup；
- 订阅解析与合并；
- HTTP 连接重试、完整下载、诊断报告路径和权限；
- CLI 参数互斥与错误传播。

Go 测试中的 sing-box JSON 断言只验证 Steer 的生成结构。目标 sing-box 1.13 是否接受配置，必须在 OpenWrt SDK/VM/真机层运行原生 `sing-box check`。

## Node.js 回归测试

```sh
node tests/node/share_url_test.js
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
```

- `share_url_test.js`：支持的分享 URI、Base64 订阅和非法输入；
- `luci_view_test.js`：表单字段、Route detour 选择、节点/路由测试、概览测试卡和 RPC 参数；
- `steer_helper_test.js`：Apply/status/诊断交互，确认概览不暴露 rollback 操作。

这些测试使用最小 LuCI mock，不等于真实浏览器渲染。发布前仍需在目标 LuCI 检查页面加载、保存、Apply、错误显示和三类测试结果。

## Python 边界检查

```sh
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
```

- i18n 检查要求所有 LuCI 消息有简体中文翻译；
- package boundaries 检查版本、依赖、conffile、schema 6→7 迁移、旧字段/日志迁移和 RPC 注册；
- build cache 检查官方 SDK digest、固定 feeds、缓存和最小包依赖闭包。

## Fixture

`tests/fixtures` 提供不会访问真实凭据的固定配置和 Geo 数据：

- `m1-openwrt-direct-valid`：VM Apply 使用的纯直连配置；
- `m1-representative-valid`：覆盖多协议、DNS 与 Geo 编译；
- `schema7-detour-valid`：两个 SOCKS 节点组成的前置代理链，用于隔离验证 `exit -> front` 编译，不要求生产路由器真实拥有这条链；
- `m1-geodata`：最小 GeoSite/GeoIP fixture。

文档地址和保留网段只用于离线配置检查，不表示可以建立真实连接。

## OpenWrt VM 集成

```sh
OPENWRT_HOST=<test-vm> \
SING_BOX_BIN=/usr/bin/sing-box \
tests/integration/run-openwrt-vm.sh
```

脚本会修改 VM 的 UCI、运行目录、nftables、策略路由和 procd，只能用于一次性测试环境。它覆盖代表配置的原生 sing-box 检查、Apply/health/status、DNS/MAC 辅助规则、RPC 注册、失败候选和显式 rollback。

如果 VM 没有独立 LAN veth 客户端，结果不覆盖真实 LAN 双栈、UDP/QUIC、GUA、flow offload 或跨接口源 MAC 行为。

## 发布验证顺序

准备提交时执行：

```sh
cd steer-openwrt/src && go test ./...
cd ../../
node tests/node/share_url_test.js
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
git diff --check
```

推送后等待固定 SDK 构建成功；tag 后等待 prerelease 发布；最后从 Release 资产安装真机。真机至少验证版本、schema 迁移、Apply/health、概览三项测试、一个裸节点和一条已有单节点 Route 的连接/下载测试。

生产配置没有多级 detour 时，不得为了测试改真实路由。把 `schema7-detour-valid` 复制到临时路径，运行 `steer prepare --config <fixture> --run-dir <temporary> --state-dir <temporary>`，由目标 sing-box 原生检查隔离候选即可。
