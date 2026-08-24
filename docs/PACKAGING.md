# 打包与发布

Steer 采用三条相互校验的交付链：定时 Geo 工作流发布当前 SRS；master 构建 OpenWrt APK 与通用 Linux 归档；tag 发布只复用该 commit 已成功生成的 master 构件。目标设备不再安装 geoview，也不再持有或转换 DAT。

## 共同规则

1. 主仓库发布 source tag、x86_64/aarch64 Linux tar.zst 和 OpenWrt 25.12.5 x86_64 APK；不构建 deb、rpm、pkg.tar 或 Nix 包。
2. Geo 工作流每 6 小时检查 Loyalsoldier 最新 release，以固定版本的生成器和 sing-box 把完整 GeoSite/GeoIP 转换成 SRS，并只保留 Pages 上的 `geodata/latest`。
3. 每份 seed 都有严格 manifest，记录上游版本、DAT SHA-256、转换工具版本以及每个 selector 对应 SRS 的路径、大小和 SHA-256。
4. 设备 Apply 只校验所引用的 seed 文件；sing-box 通过 `initial_path` 立即启动，并使用 direct HTTP client 每 24 小时后台检查同名 remote SRS。
5. 控制器要求 `sing-box >=1.14.0-beta.2,<1.15.0`。当前发布构建和系统验收固定使用官方 `1.14.0-rc.1`。
6. tag 发布不重新编译，只复用 tag 所指 master commit 的成功 release bundle。预发布 tag 创建 GitHub prerelease，但不替换稳定 OpenWrt 软件源。

当前预览版本是 `v0.7.0-alpha.1`。alpha 描述的是 Steer 的稳定性，不改变 sing-box 的独立版本语义。

## Geo SRS

`.github/workflows/geodata.yml` 是唯一生产转换链：

```text
Loyalsoldier geosite.dat + geoip.dat
                  ↓
固定 geoview library + 固定 sing-box compiler
                  ↓
完整 rules/*.srs + manifest.json
                  ↓
逐文件反编译、非空检查、大小与 SHA-256 验证
                  ↓
GitHub Pages geodata/latest
```

这里的 geoview 仅是 CI 生成器的 Go 构建依赖，不是 Steer 或目标设备的运行时依赖。manifest 是 selector 的唯一清单，包含基础 category 及 `category@attribute`；已从上游删除的 selector 会在下一次 Apply 明确失败，不做静默 fallback。

公开地址：

```text
https://gsh20040816.github.io/steer/geodata/latest/manifest.json
https://gsh20040816.github.io/steer/geodata/latest/rules/steer-geosite-cn.srs
https://gsh20040816.github.io/steer/geodata/latest/steer-geodata.tar.zst
```

Pages 只保存当前版本，不承诺历史 seed 或可重复取得任意旧上游输入。master 构建先下载、展开并完整验证一个确定的当前 bundle，再把同一 seed 嵌入该次 OpenWrt 和 Linux 产物；构件元数据记录其版本和 manifest SHA-256。

## OpenWrt

`v0.7.0-alpha.1` 面向 OpenWrt 25.12.5 x86/64：

| 包 | 版本 | 所有内容 |
|---|---|---|
| `steer` | `0.7.0_alpha.1-r1` | 控制器、默认 UCI、procd init、完整只读 SRS seed |
| `luci-app-steer` | `0.7.0_alpha.1-r1` | LuCI 页面、ucode RPC、ACL |
| `luci-i18n-steer-zh-cn` | `0.7.0_alpha.1-r1` | 简体中文翻译 |
| `sing-box` | `1.14.0_rc1-r0` | SagerNet 官方 x86_64 APK，经内容核验后改用 Steer 仓库密钥签名 |

Steer 不重编译 sing-box。CI 先校验官方 APK 的固定 SHA-256，重签后比较除签名记录外的 APK 元数据，再用仓库公钥验证安装包。软件源的四个 APK 与 `packages.adb` 使用同一 Steer P-256 信任根；私钥只来自 GitHub Actions Secret `OPENWRT_APK_PRIVATE_KEY`，不得进入源码、构件、Release 或 Pages。

控制器依赖 `firewall4`、`ip`、`kmod-tun`、`kmod-nft-queue`、`kmod-nft-tproxy` 和上述 sing-box 版本范围，不依赖 geoview 或独立 Geo 包。

持久与易失状态：

```text
/etc/config/steer                    用户 UCI；包 conffile
/usr/share/steer/geodata-seed        包拥有的只读 SRS seed 与 manifest
/var/lib/steer/cache.db              sing-box remote SRS 与可选 DNS cache
/var/lib/steer/subscriptions         订阅快照
/var/lib/steer/logs/tests            最新测试报告
/run/steer                           generation、current、last_apply、锁
```

安装后脚本显式执行唯一的 schema 7→8 迁移，维护订阅 cron，再运行正常 `steer apply`。迁移只删除旧 DNS profile 上无法表达的 cache 字段并提升 schema；非法或非 schema 7/8 配置会立即失败。包仍通过 `PROVIDES`、`CONFLICTS` 和 `REPLACES` 原子替换历史 `steer-openwrt` 包名，但源码中不保留兼容实现。

### 稳定软件源

稳定 tag 发布成功后，`publish.yml` 才把同一 master 构建产生的四个 APK、签名索引、公钥和校验元数据部署到 GitHub Pages。预发布不会替换这里的 OpenWrt 内容；独立 Geo 工作流仍会保留该目录并只更新 `geodata/latest`。

```text
https://gsh20040816.github.io/steer/openwrt/25.12.5/x86_64/packages.adb
https://gsh20040816.github.io/steer/openwrt/25.12.5/x86_64/steer-apk.pem
```

OpenWrt 端先安装公钥，再添加索引：

```sh
wget -O /etc/apk/keys/steer-apk.pem \
  https://gsh20040816.github.io/steer/openwrt/25.12.5/x86_64/steer-apk.pem
echo 'https://gsh20040816.github.io/steer/openwrt/25.12.5/x86_64/packages.adb' \
  >> /etc/apk/repositories.d/customfeeds.list
apk update
apk add steer luci-app-steer luci-i18n-steer-zh-cn
```

不得以 `--allow-untrusted` 代替密钥安装。轮换私钥时必须发布新文件名，并先完成设备信任迁移。

## 通用 Linux

GitHub Release 提供：

```text
steer-linux-x86_64.tar.zst
steer-linux-aarch64.tar.zst
```

每个归档结构固定为：

```text
steer-linux-<arch>/
├── steer
├── systemd/
│   ├── steer.service
│   ├── steer-web.service
│   ├── steer-subscription.service
│   └── steer-subscription.timer
├── config.example.json
├── web.example.json
├── geodata-seed/
│   ├── manifest.json
│   └── rules/*.srs
└── LICENSE
```

归档不包含 sing-box、geoview 或 DAT。目标系统必须提供 systemd、匹配版本的 sing-box、nftables、iproute2 和 ca-certificates。发行版维护者从固定 source tag 构建 `./go/cmd/steer-linux`，安装为 `/usr/bin/steer`，并原样安装已经验证的 seed。

## Arch Linux AUR

本仓库只维护一个源码配方：

```text
packaging/archlinux/steer/
├── PKGBUILD
├── .SRCINFO
├── steer.install
└── .gitignore
```

配方从固定 `_commit` 构建 Steer，并下载 Pages 当前 Geo bundle；`prepare()` 使用同一 Go verifier 完整校验 seed。运行依赖是 `sing-box>=1.14.0beta2`、`sing-box<1.15.0`、systemd、nftables、iproute2 和 ca-certificates。安装/升级 hook 自动执行 schema 7→8 迁移，但不启用服务、不 Apply 配置，也不生成 Web token。

`PKGBUILD` 是唯一手工维护的配方，`.SRCINFO` 必须由 `makepkg --printsrcinfo` 生成。更新时：

1. 更新 `pkgver` 和 40 位 `_commit`；仅打包修订才只增加 `pkgrel`。
2. 重新生成 `.SRCINFO` 并确认与 `PKGBUILD` 一致。
3. 在干净 Arch 环境运行 `makepkg --cleanbuild --syncdeps`，检查文件、权限、版本、seed 和依赖。
4. 人工复制并审查包目录后，再推送独立 AUR Git 仓库；CI 不保存 AUR 凭据。

## 主线构建与发布门

```text
verify
  ├── Go race tests + vet
  ├── Node/LuCI tests
  └── i18n、包边界、工作流与 Linux 打包契约检查
        ↓
verified Pages Geo seed
  ├── OpenWrt SDK 构建三个 Steer/LuCI APK
  │     + 校验并重签官方 sing-box APK
  │     + 签名 packages.adb
  └── CGO_ENABLED=0 构建两份 Linux tar.zst
        ↓
Linux systemd 容器验收
        ↓
verified release bundle
```

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
git diff --check
```

tag 流程：

1. 推送 master 原子变更并等待对应 `Build release artifacts` 成功。
2. 给同一 commit 打版本 tag 并推送。
3. `Publish tagged release` 查找该 commit 的成功 push run，下载并再次验证 release bundle 与 OpenWrt repository。
4. 创建 GitHub Release；稳定 tag 才更新 Pages OpenWrt 软件源，预发布只创建 prerelease。

最终 Release 包含四个 APK、两个 Linux tar.zst、`BUILD-METADATA.txt` 和 `SHA256SUMS`。任何 tag 都不得重新构建或混用其他提交的产物。
