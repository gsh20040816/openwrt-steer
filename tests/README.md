# 测试说明

测试按共享核心、OpenWrt/Linux/macOS 适配器、LuCI/Linux Web/macOS GUI 和目标系统正常路径分层。当前范围不包含故障注入矩阵。

## Go

```sh
cd go
go test -race ./...
go vet ./...
```

覆盖 schema 9、严格 JSON/UCI 解码、引用和前置链校验、确定性编译、Route 私有出站、DNS 路径、remote SRS/seed manifest、共享 Apply 生命周期、generation、订阅合并/Store、probe 测量，以及 OpenWrt 计划、nftables、激活、健康和日志。

Linux 适配器测试覆盖主机与转发流量 plan、OUTPUT/PREROUTING DNS shim 与受保护的 wildcard listener、1.14 原生 source-MAC、JSON 原子写入与 ETag 冲突、systemd/backend generation、Web bearer token/CSP/开关失败回滚、临时 probe 的 bypass mark 和静态 Linux 构建。

macOS 适配器测试覆盖 Darwin TUN plan、TUN port-53 capture、JSON store、generation、launchd backend 和平台限制；`check-macos-contract.py` 约束 SwiftUI GUI 直接面向 helper，并确保旧数据面实验路径不会重新进入仓库。

## LuCI 与静态边界

```sh
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
node tests/node/linux_web_test.js
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
python3 tests/check-linux-packaging.py
python3 tests/check-macos-contract.py
python3 tests/check-macos-packaging.py
python3 tests/check-ui-contract.py
```

- `internal/subscription` Go 测试：三端共享的代理 URI、多行与 Base64 解析；
- `luci_view_test.js`：表单语义、detour 可清空、节点/路由/概览测试按钮；
- `steer_helper_test.js`：UCI commit 后的 Apply 观察、独立 validate、最小 status；
- `linux_web_test.js`：Linux Web 开关失败/冲突回滚、Rules/Node chips 原子提交与 SSH 私钥 DOM/换行往返；
- Python 检查：中文覆盖、包所有权/版本/已删除接口、官方 SDK 缓存与 Linux 交付约束。

## OpenWrt VM

`tests/integration/run-openwrt-vm.sh` 只能运行在一次性 OpenWrt 25.12.5 x86/64 VM，并要求匹配版本的 sing-box、完整 SRS seed 和待测控制器已准备。脚本会改写 `/etc/config/steer`、`/run/steer`、nftables、策略路由和 procd，不能用于生产路由器。

它覆盖公共 RPC 集、Geo catalog、代表配置校验、正常 Apply、LuCI UCI commit 触发、health/status、DNS shim 与 1.14 原生 MAC 规则、fw4 reload、服务 restart/reload、非法字段 fail-fast、禁用和重新启用。编译器的详细结构由 Go 测试覆盖，不为测试重新公开 compile/plan/prepare 命令。

## Linux systemd 容器

`tests/integration/run-linux-system.sh` 只能运行在显式设置 `STEER_LINUX_SYSTEM_TEST=1` 的一次性 privileged systemd 容器，不能用于生产主机。发布 CI 使用 `tests/integration/linux-system.Dockerfile` 构建固定 Debian 环境，挂载本次构建的 Steer、同次验证的 SRS seed 和校验过 SHA-256 的 sing-box 1.14.0-rc.1 musl 二进制。

脚本建立 upstream/client 两个 netns，固定 client MAC 后先验证隔离拓扑，再把默认路由切到无公网出口的 upstream。启用配置实际引用 `geosite:cn` 和 native `source_mac_address`，所以服务在 Pages 不可达时仍必须通过包内 `initial_path` 启动；随后覆盖主机和转发流量的 IPv4/IPv6 TCP、UDP、UDP/TCP53。DNS 请求使用不存在的原目标地址，只有经过 Steer redirect 和独立 DNS upstream 才能成功；测试同时检查 nft DNS counter 增长，并确认 `steer0` 没有被注册为 systemd-resolved DNS route。它还确认 1053/1054 不能被直接当作 LAN resolver 访问，并覆盖服务重启、`nftables.service` 重启、禁用和重新启用。

## 发布前完整检查

```sh
cd go && go test -race ./... && go vet ./...
cd ..
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
node tests/node/linux_web_test.js
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
python3 tests/check-linux-packaging.py
sh -n tests/integration/run-openwrt-vm.sh
sh -n tests/integration/run-linux-system.sh
git diff --check
```

所有分支 commit 与 PR 都运行 Ubuntu/Linux/OpenWrt 测试、Linux systemd 容器集成，以及 arm64/x86_64 原生 macOS Go/Swift 测试；CI 不设置 concurrency 限制，也不保存正式发布包。只有 tag commit 已进入 master 且同一 SHA 的 master CI push run 成功后才允许发布；OpenWrt SDK、Linux 归档、原生 macOS DMG、attestation 和发布全部在同一次 tag workflow 中完成。
