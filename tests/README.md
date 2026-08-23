# 测试说明

测试按共享核心、OpenWrt/Linux 适配器、LuCI 和目标系统正常路径分层。当前范围不包含故障注入矩阵。

## Go

```sh
cd go
go test -race ./...
go vet ./...
```

覆盖 schema 7、严格 JSON/UCI 解码、引用和前置链校验、确定性编译、Route 私有出站、DNS 路径、共享 Apply 生命周期、generation、订阅合并/Store、probe 测量，以及 OpenWrt 计划、nftables、激活、健康、Geo、日志和 UCI batch。

Linux 适配器测试覆盖主机与转发流量 plan、OUTPUT/PREROUTING DNS shim 与受保护的 wildcard listener、JSON 原子写入与 ETag 冲突、Linux source-MAC 拒绝、systemd/backend generation、Web bearer token/CSP/开关失败回滚、临时 probe 的 bypass mark 和静态 Linux 构建。

## LuCI 与静态边界

```sh
node tests/node/share_url_test.js
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
node tests/node/linux_web_test.js
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
python3 tests/check-linux-packaging.py
```

- `share_url_test.js`：代理 URI 的解析和告警；
- `luci_view_test.js`：表单语义、detour 可清空、节点/路由/概览测试按钮；
- `steer_helper_test.js`：UCI commit 后的 Apply 观察、独立 validate、最小 status；
- `linux_web_test.js`：Linux Web 开关保存失败与 revision conflict 的草稿回滚；
- Python 检查：中文覆盖、包所有权/版本/已删除接口、官方 SDK 缓存与 Linux 交付约束。

## OpenWrt VM

`tests/integration/run-openwrt-vm.sh` 只能运行在一次性 OpenWrt 25.12.5 x86/64 VM，并要求 sing-box、geoview、Geo seed 和待测控制器已准备。脚本会改写 `/etc/config/steer`、`/run/steer`、nftables、策略路由和 procd，不能用于生产路由器。

它覆盖公共 RPC 集、Geo catalog、代表配置校验、正常 Apply、LuCI UCI commit 触发、health/status、DNS/MAC 辅助层、fw4 reload、服务 restart/reload、非法字段 fail-fast、禁用和重新启用。编译器的详细结构由 Go 测试覆盖，不为测试重新公开 compile/plan/prepare 命令。

## Linux systemd 容器

`tests/integration/run-linux-system.sh` 只能运行在显式设置 `STEER_LINUX_SYSTEM_TEST=1` 的一次性 privileged systemd 容器，不能用于生产主机。发布 CI 使用 `tests/integration/linux-system.Dockerfile` 构建固定 Debian 环境，挂载本次构建的 Steer 和校验过 SHA-256 的 sing-box 1.13.19 musl 二进制。

脚本建立 upstream/client 两个 netns，先验证隔离拓扑，再覆盖主机和转发流量的 IPv4/IPv6 TCP、UDP、UDP/TCP53；DNS 请求使用不存在的原目标地址，只有经过 Steer redirect 和独立 DNS upstream 才能成功。它还确认 1053/1054 不能被直接当作 LAN resolver 访问，并覆盖服务重启、`nftables.service` 重启、禁用和重新启用。

## 发布前完整检查

```sh
cd go && go test -race ./... && go vet ./...
cd ..
node tests/node/share_url_test.js
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

官方 SDK 构建、Linux 通用归档和 Linux systemd 容器验收共同构成最终门禁；只有同一 commit 的 master 构建成功后才允许打发布 tag。
