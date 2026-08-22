# 打包与发布

0.4.2 面向 OpenWrt 25.12.5 x86/64。GitHub Actions 使用固定官方 SDK 镜像构建；tag 发布只复用同一 commit 已成功生成的 master 构建，不重新编译。

## 包与所有权

| 包 | 版本 | 所有内容 |
|---|---|---|
| `steer-openwrt` | `0.4.2-r1` | `/usr/sbin/steer`、默认 UCI、procd init、Apply/OpenWrt 适配器 |
| `luci-app-steer` | `0.4.2-r1` | LuCI 页面、ucode RPC、ACL |
| `luci-i18n-steer-zh-cn` | `0.4.2-r1` | 简体中文翻译 |
| `steer-geodata` | 独立时间版本 | 固定 GeoSite/GeoIP seed |
| `geoview` | 固定上游 commit | Geo 分类读取工具 |

`sing-box` 由 OpenWrt package manager 提供，Steer 不安装或替换它。控制器只支持 `sing-box >= 1.13.18 且 < 1.14.0`，并依赖 `firewall4`、`ip`、`kmod-tun`、`kmod-nft-queue`、`kmod-nft-tproxy`、`geoview` 和 `steer-geodata`。APK 依赖与运行时 capability 检查使用同一版本边界，包管理器会在安装阶段拒绝 1.14 及更高版本。

Go 源码位于仓库根的 `go/`，包定义从同一 feed 的兄弟目录复制构建输入。旧 `steer-openwrt/src` 不存在，也没有兼容副本。

## 持久与易失状态

```text
/etc/config/steer                    用户 UCI；包 conffile
/usr/share/steer/geodata-seed        包拥有的只读 Geo 输入
/var/lib/steer/geodata               生成的 SRS
/var/lib/steer/subscriptions         订阅快照
/var/lib/steer/logs/tests            最新测试报告
/run/steer                           generation、current、last_apply、锁
```

安装后脚本只接受 schema 7，确认 BusyBox crond 可用，维护订阅 cron，并执行一次正常 `steer apply`。它不迁移旧 schema、不补测试 URL、不改节点/路由。已经退出的 `/var/lib/steer/rollback.uci` 会被显式删除。

## 构建

主线 push 运行：Go 全量测试、Node/LuCI 回归、i18n/包边界/缓存检查，以及官方 SDK 构建。SDK 产出收集到 `dist/`，包含五个 APK、构建元数据和 SHA256SUMS。

本地可先运行：

```sh
cd go && go test ./...
cd ..
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
node tests/node/share_url_test.js
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
```

官方 SDK 容器命令以 `.github/workflows/release.yml` 和 `.github/actions/openwrt-sdk/entrypoint.sh` 为准，不维护第二份手工构建流程。

## 发布

APK 版本为 `0.4.2-r1`，Git tag 为 `v0.4.2`。发布流程为：

1. 在 master 提交并推送完整原子变更；
2. 等待该 commit 的 `Build OpenWrt packages` 成功；
3. 给同一 commit 打 `v0.4.2` tag 并推送；
4. `Publish tagged release` 下载 master 构建、校验 SHA256/commit，再创建正式 Release；
5. 从 Release 下载 APK 安装到目标路由器，执行正常配置校验和健康检查。

不得从本地未留证据的二进制发版，也不得给 tag 重新构建一套可能不同的产物。`v0.4.2` 复用同一提交已经成功生成并留存证据的 master 构建。
