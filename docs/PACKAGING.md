# 打包与文件所有权

## 原则

- 软件、第三方二进制和规则数据必须由系统包管理器安装、升级和删除。
- Steer 不在运行时下载文件并覆盖 `/usr`，不从网络替换包管理器拥有的文件。
- 第三方依赖独立成包；Steer 主包只声明依赖，不复制 sing-box、SmartDNS 或 geoview 二进制。
- `/usr` 是只读发布输入，`/etc` 是用户配置，`/var/lib/steer` 保存可重建 Geo 派生物和唯一的
  `rollback.uci`，`/run/steer` 是临时运行状态。
- 生成物不作为包更新输入；删除生成物后必须能从包和配置重新构建。

## 当前 OpenWrt 包

| 包 | 拥有内容 | 不得拥有 |
|---|---|---|
| `steer-openwrt` | Go 控制程序、OpenWrt adapter、UCI、procd/firewall4 挂接 | 第三方二进制、远端规则数据 |
| `luci-app-steer` | LuCI 页面、RPC 和 ACL | 核心运行时、规则数据 |
| `steer-geodata` | 固定版本、固定哈希的 GeoSite/GeoIP 源数据 | updater、调度器、运行状态 |
| `geoview` | 固定上游源码构建的独立转换工具 | Steer 配置与状态 |
| 发行版 `sing-box` | sing-box 二进制 | Steer 配置与状态 |
| 发行版其他依赖 | firewall4、ucode 等各自文件 | Steer 配置与状态 |

`steer-geodata` 安装到 `/usr/share/steer/geodata-seed`。Steer 可以把其中的数据转换为
`/var/lib/steer/geodata` 下的 sing-box `.srs`，但 GeoSite/GeoIP 版本升级只能通过安装新版
`steer-geodata` 包完成。包升级后，下一次 Apply 根据 package release 重新生成派生规则；如果
新数据缺少被引用分类，Apply 必须失败并保留当前运行代。

## M1 包布局

- `steer-openwrt`：按需运行的静态 Go 控制程序 `/usr/sbin/steer` 及 OpenWrt 生命周期适配；
- `luci-app-steer`：只依赖 `steer-openwrt`；
- `steer-geodata`：独立版本化数据包；
- `geoview`：独立转换工具。

不创建独立 `steer-core` APK。平台中立的 Go 包与 OpenWrt adapter 编译进同一个
`steer-openwrt` 二进制；这不妨碍后续其他平台复用源码，也不制造没有独立生命周期价值的包。

`steer-openwrt` 必须声明替换并冲突旧 `steer`，保留 `/etc/config/steer`，且只在 schema 6
preflight 通过时启用服务。schema 5→6 的迁移窗口已经关闭，当前包要求 schema 6，未知 schema
直接失败。本次 release 仅修复上一版 VMess 订阅错误写入的冗余 `network` 字段；下一包必须删除
这段精确修复，不保留通用兼容层，也不自动降级 APK。
包升级期间允许旧 init 脚本按 OpenWrt 约定停止；新包必须先完成唯一一次 UCI 迁移，再显式启动
新 generation。不能让 `default_postinst` 在迁移前启动服务，否则旧 schema 会导致配置解码失败，
并留下“包已升级、数据面未启动”的半完成状态。启动失败必须让包事务失败并暴露错误。

## 发布约束

- OpenWrt Makefile 中可安全选择的运行依赖使用 `DEPENDS`。OpenWrt 25.12 的 `curl/libcurl` 与
  `sing-box/sing-box-tiny` 存在上游 Kconfig 递归选择，暂以标准 `EXTRA_DEPENDS` 写入 APK 依赖
  元数据，但不把其运行变体变成 Steer 的构建选项；不得借此隐藏其他可正常解析的依赖。
- 下载发生在包构建阶段，必须固定版本、源码提交和哈希。
- Release 只发布 CI 从锁定 SDK 构建的独立 APK、SHA-256、元数据和日志。
- 不允许通过安装脚本、LuCI RPC 或后台调度器实现自更新。

## CI 依赖缓存

Release 工作流直接使用固定 digest 的官方 OpenWrt 25.12.5 SDK 镜像。`base` feed 保留
官方 SDK 的 `--root=package` 布局，`packages`、`luci` 和本地 Steer feed 均固定来源。
不再维护完整 OpenWrt 源码、外部 toolchain 或外部 Go bootstrap 的自制 builder。

第一次 `make defconfig` 前必须关闭 `CONFIG_ALL_KMODS` 与
`CONFIG_ALL_NONSHARED`；安装 Steer feed 并选择 `luci-app-steer` 后再执行最终
`defconfig`。最终配置必须再次确认这两个开关关闭，并拒绝 r8169、video 等不属于
Steer 依赖闭包的模块。构建只提交一个 `package/luci-app-steer/compile` 顶层目标，
由 OpenWrt 解析并构建 `geoview`、`steer-geodata`、`steer-openwrt`、LuCI 应用和中文
i18n 包。feeds 更新与每个 package download 保持串行，只有 compile 使用并行 make。

GitHub Actions 只持久化两个可重建缓存：SDK 原生 `/builder/.ccache` 和
OpenWrt Go package 使用的 GOCACHE。`dl` 每次由 GitHub runner 重新下载；`build_dir`、
`staging_dir`、hostpkg stamp、包安装状态和其他 target state 一律不缓存。
入口脚本使用 `make val.CCACHE_DIR` 核对 OpenWrt 实际解析到
`/builder/.ccache`，路径不一致立即失败；ccache 的 compiler check 同时绑定 SDK
版本与镜像 digest。

GOCACHE 只复用 Go 编译对象，不能提供 GOROOT，也不能替代 OpenWrt
`golang/host` 的安装。官方 SDK 在全新工作区中仍会依次构建 Go 1.24.13 bootstrap
和 Go 1.26.4 host toolchain；缓存命中只能缩短其中可缓存的编译，不能把这条工具链
依赖伪装成已经消失。

Steer 的策略路由只使用 `ip rule` 与 `ip route` 的 IPv4/IPv6 基础操作，因此依赖 OpenWrt
的虚拟 `ip` provider。全新安装由 OpenWrt 的默认 variant 选择 `ip-tiny`，从而保持最小安装
闭包；已经因 PassWall2、SQM 或其他软件安装 `ip-full` 的设备则直接复用现有 provider，不能
强制换成与它冲突的 `ip-tiny`。OpenWrt 当前仍按 `iproute2` 源包级依赖调度构建，因此即使
最终配置未选择 `ip-full`、`libbpf` 和 `libelf`，构建日志中仍会出现 libbpf、elfutils 和
gettext；这些包不得仅由 Steer 引入为 APK 运行依赖。该边界由 OpenWrt VM 集成测试中的双栈
fwmark rule、local route、清理与实际透明代理流量验证。
