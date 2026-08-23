# 打包与发布

Steer 固定采用三层交付模型：主仓库发布 source tag、通用 Linux 归档和 OpenWrt APK；发行版维护者负责 AUR、deb、rpm 等下游包；运行时依赖和 Geo 数据由目标系统的包管理器或用户提供。主仓库不充当通用 Linux 发行版仓库。

## 共同规则

1. 主仓库的自动构建只产出平台中立的 Linux tar.zst，不构建 deb、rpm、pkg.tar、Nix 等发行版包；Arch AUR 的源码配方作为人工维护的下游元数据提交在本仓库。
2. OpenWrt 作为一级 appliance 目标，继续由主仓库使用固定官方 SDK 构建 APK。
3. Linux 和 OpenWrt 的安装命令都叫 `steer`；`steer-linux`、`steer-openwrt` 只用于区分源码 target。
4. sing-box 始终是外部依赖，不进入任何 Steer 产物。
5. geoview 是未打补丁的上游依赖；Linux 归档不捆绑它。
6. OpenWrt 通过 `steer-geodata` 提供固定 Geo 输入；Linux 只消费用户配置路径指向的兼容 `.dat`，不依赖任何特定 Geo 包。
7. tag 发布不重新编译，只复用该 tag 所指 master commit 已成功生成的完整 release bundle。

版本所有权彼此独立：Steer、LuCI 和 OpenWrt `steer` APK 跟随 `v0.6.5`；`steer-geodata` 使用独立数据版本；geoview 和 sing-box 跟随各自上游或发行版。

## OpenWrt

0.6.5 面向 OpenWrt 25.12.5 x86/64：

| 包 | 版本 | 所有内容 |
|---|---|---|
| `steer` | `0.6.5-r1` | `/usr/sbin/steer`、默认 UCI、procd init、Apply/OpenWrt 适配器 |
| `luci-app-steer` | `0.6.5-r1` | LuCI 页面、ucode RPC、ACL |
| `luci-i18n-steer-zh-cn` | `0.6.5-r1` | 简体中文翻译 |
| `steer-geodata` | 独立时间版本，`r2` | 固定 GeoSite/GeoIP 输入文件 |
| `geoview` | `0.2.6-r3` | 固定上游 commit、无下游补丁的 Geo 分类读取工具 |

`sing-box` 由 OpenWrt package manager 提供。控制器要求 `sing-box >= 1.13.18 且 < 1.14.0`，并依赖 `firewall4`、`ip`、`kmod-tun`、`kmod-nft-queue`、`kmod-nft-tproxy`、`geoview` 和 `steer-geodata`。

持久与易失状态：

```text
/etc/config/steer                    用户 UCI；包 conffile
/usr/share/steer/geodata-seed        包拥有的只读 Geo 输入
/var/lib/steer/geodata               生成的 SRS
/var/lib/steer/subscriptions         订阅快照
/var/lib/steer/logs/tests            最新测试报告
/run/steer                           generation、current、last_apply、锁
```

安装后脚本只接受 schema 7，维护订阅 cron，并执行一次正常 `steer apply`；它不迁移旧 schema、不改节点/路由。OpenWrt 包仍通过 `PROVIDES`、`CONFLICTS` 和 `REPLACES` 原子替换历史 `steer-openwrt` 包名，但源码中不保留兼容目录。

## 通用 Linux

主仓库发布：

```text
steer-linux-x86_64.tar.zst
steer-linux-aarch64.tar.zst
```

归档结构固定为：

```text
steer-linux-<arch>/
├── steer
├── systemd/
│   ├── steer.service
│   ├── steer-web.service
│   ├── steer-subscription.service
│   └── steer-subscription.timer
├── config.example.json
├── platform.example.json
├── web.example.json
└── LICENSE
```

归档不包含 sing-box、geoview、geosite.dat 或 geoip.dat。目标系统必须提供 systemd、sing-box、geoview、nftables、iproute2 和 ca-certificates；Geo 文件只有在 Intent 实际引用对应 kind 时才需要。`platform.example.json` 只让用户配置 GeoSite/GeoIP 文件路径，不公开或要求配置内容哈希。

发行版维护者应从固定 source tag 构建 `./go/cmd/steer-linux`，安装为 `/usr/bin/steer`，安装 `linux/systemd/` 和示例配置，并声明上述外部依赖。Arch 应由独立 AUR `steer` 源码包维护；若发行版没有 geoview，应由独立的无 Steer 前缀的 geoview 包提供。

## Arch Linux AUR

主仓库只维护两个 AUR 源码配方：

```text
packaging/archlinux/
├── steer/
│   ├── PKGBUILD
│   ├── .SRCINFO
│   └── .gitignore
└── geoview/
    ├── PKGBUILD
    ├── .SRCINFO
    └── .gitignore
```

`steer` 从对应稳定版的固定 source commit 构建，因此构建依赖同时声明 `git` 和 `go`；运行时依赖发行版提供的 `sing-box >=1.13.18,<1.14.0`、`geoview`、systemd、nftables、iproute2 和 ca-certificates。`geoview` 直接从其独立的上游 release 构建；0.2.6 源码没有声明许可证，因此配方按 Arch 规范标记为 `unknown`，不从其他发行版的打包元数据推测上游授权。两者都不捆绑 sing-box 或 Geo 数据，也不在安装过程中启用服务、Apply 配置或生成 Web token。Linux Geo 数据保持 provider-neutral，不声明特定 Geo 包为依赖。

这里的 `PKGBUILD` 是唯一手工维护的配方，`.SRCINFO` 只是由它生成并随包目录提交的元数据；独立的 AUR Git 仓库只是人工发布镜像。当前不在 CI 中保存 AUR 凭据或推送 AUR，也不提供同步/推送脚本。更新时按以下顺序执行：

1. 更新稳定版时同时更新 `pkgver` 和 40 位 `_commit`；配方通过 `git+https://...#commit=$_commit` 克隆不可移动的 source revision。纯打包修订只增加 `pkgrel`。VCS source 本身使用 `sha256sums=('SKIP')`，供应链边界由固定 commit、HTTPS 和上游仓库共同确定。
2. 在对应目录用 `makepkg --printsrcinfo` 重新生成 `.SRCINFO`，不得手工维护其字段，并确认生成结果与提交内容完全一致。
3. 在干净 Arch 环境先构建 geoview，再构建 steer；分别执行 `makepkg --cleanbuild --syncdeps`，检查生成包的文件、权限、版本与依赖。
4. 提交主仓库后，再由维护者把该包目录中的 `PKGBUILD`、`.SRCINFO` 和 `.gitignore` 人工复制、审查并提交到对应 AUR Git 仓库。

## 主线构建

`.github/workflows/release.yml` 的单次 master push 分为：

```text
verify
  ├── Go race tests + vet
  ├── Node/LuCI tests
  └── i18n、包边界、工作流与 Linux 打包契约检查
        ├── openwrt：固定官方 SDK → 五个 APK
        └── linux：CGO_ENABLED=0 → x86_64/aarch64 tar.zst
                         ↓
                verified release bundle
```

OpenWrt 与 Linux 在同一 verify 之后并行构建。`scripts/collect-openwrt-artifacts.sh` 只收集 APK；`scripts/collect-linux-artifacts.sh` 只组装通用 Linux 归档。bundle job 分别验证平台 SHA256 和源码提交，再生成统一的 `BUILD-METADATA.txt` 与 `SHA256SUMS`。

本地完整验证至少包括：

```sh
cd go
go test -race ./...
go vet ./...
cd ..
python3 tests/check-luci-i18n.py
python3 tests/check-package-boundaries.py
python3 tests/check-build-cache.py
python3 tests/check-linux-packaging.py
node tests/node/share_url_test.js
node tests/node/luci_view_test.js
node tests/node/steer_helper_test.js
node tests/node/linux_web_test.js
sh -n tests/integration/run-openwrt-vm.sh
sh -n tests/integration/run-linux-system.sh
```

## Tag 发布

稳定版流程为：

1. 在 master 提交并推送完整原子变更；
2. 等待该 commit 的 `Build release artifacts` 成功，确认 OpenWrt SDK、Linux 通用归档、Linux systemd 容器验收和 release bundle 全部通过；
3. 给同一 commit 打 `v0.6.5` tag 并推送；
4. `Publish tagged release` 查找该 commit 的成功 master run，下载 `release-bundle`，再次验证 SHA256、commit 和精确资产集合；
5. 发布 GitHub Release；
6. 从 Release 重新下载 APK 安装到目标 OpenWrt，并执行配置哈希、Apply、健康与 DNS 验证。

最终 Release 包含五个 APK、两个 Linux tar.zst、`BUILD-METADATA.txt` 和 `SHA256SUMS`。任何 tag 都不得重新构建或混用其他提交的产物。
