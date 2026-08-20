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

`steer-openwrt` 必须声明替换并冲突旧 `steer`，保留 `/etc/config/steer`，且只在 schema 4
preflight 通过时启用服务。包升级不承担配置迁移或 APK 自动降级。

## 发布约束

- OpenWrt Makefile 中可安全选择的运行依赖使用 `DEPENDS`。OpenWrt 25.12 的 `curl/libcurl` 与
  `sing-box/sing-box-tiny` 存在上游 Kconfig 递归选择，暂以标准 `EXTRA_DEPENDS` 写入 APK 依赖
  元数据，但不把其运行变体变成 Steer 的构建选项；不得借此隐藏其他可正常解析的依赖。
- 下载发生在包构建阶段，必须固定版本、源码提交和哈希。
- Release 只发布 CI 从锁定 SDK 构建的独立 APK、SHA-256、元数据和日志。
- 不允许通过安装脚本、LuCI RPC 或后台调度器实现自更新。

## CI 依赖缓存

Release 工作流遵循 OpenWrt 官方共享 CI 的缓存边界：固定提交的普通 OpenWrt
源码树运行在固定 digest 的官方 toolchain 容器中，容器通过 `/prebuilt_tools` 提供
host tools，通过 `/external-toolchain` 提供交叉工具链。工作流只持久化 package 类型
的 `.ccache`；target `build_dir`、target `staging_dir`、OpenWrt stamp 和上一次的包
安装状态一律不缓存，也不通过修改时间戳伪造依赖已完成。
缓存目录必须挂载到源码树的 `/work/openwrt/.ccache`；OpenWrt make 会将编译进程的
`CCACHE_DIR` 解析到该位置。挂载到旧 SDK 使用的 `/builder/.ccache` 只会保存一个几乎
为空的目录，而真正的编译缓存会随容器销毁。入口脚本会核对 make 的解析结果并在
路径不一致时立即失败。

构建配置明确选择 `CONFIG_CCACHE=y` 和用于一次性 CI 的 `CONFIG_AUTOREMOVE=y`。
ccache 使用官方 package 配置：`compiler_type=gcc`、8 GiB 上限、depend mode，以及
OpenWrt 官方采用的时间与 include 文件 sloppiness。固定源码提交、toolchain digest
与 target 进入 cache key；恢复成功后构建会更新 ccache，工作流按官方方式删除旧的
同 key 缓存并保存更新版本。

GitHub Actions cache 以 branch/tag ref 隔离，兄弟版本 tag 不能互相读取缓存。因此共享
Buildx cache 和 package `.ccache` 只由默认分支的可信 push 写入与轮换；版本 tag 只读
恢复默认分支缓存，不创建 tag 私有副本。默认分支和 tag 的包构建使用同一 concurrency
组串行执行，保证版本构建不会在默认分支尚未完成缓存更新时抢跑。删除旧 `.ccache` 时
还必须把 REST 请求限定到默认分支 ref，不能按 key 跨 ref 删除。

配置 external toolchain 后仍须按官方顺序运行 `make tools/install` 与
`make toolchain/install`。这两步不会重新编译容器中已有的完整工具链：前者确认并安装
`/prebuilt_tools` 对应的 host 工具状态，后者生成当前源码树使用的 compiler wrapper，
并从 `/external-toolchain` 导入 GCC 与 musl 版本信息。直接跳过会使基础库 APK 的版本
退化为 `unknown`，不是有效的“省略重复构建”。

不直接使用 `ghcr.io/openwrt/sdk` 作为构建根。该全包 SDK 的 `Config-build.in` 将
约 1190 个 kmod 固定为不可交互的 `default m`；Steer 只要声明一个内核模块依赖，
kernel package 阶段就会打包大量无关模块。普通源码树配合官方 external toolchain
保留正常 Kconfig 选择语义，最终配置只选择 `luci-app-steer` 及其依赖，并对异常的
kmod 数量和已知无关模块设置失败检查。

`ext-toolchain.sh --config x86/64` 会先带入用于完整固件镜像的 x86 默认设备包。
单包构建在此基础上清除初始 `CONFIG_PACKAGE_*` 选择，再让 Kconfig 从
`luci-app-steer` 重新选择依赖；配置中出现 target-profile firmware 会立即失败。
x86 子目标仍会把少量不可交互的默认 kmod 固定为 `y`，单包 make 调用会覆盖这些
`=y` 项为空，只构建 Steer 依赖选择为 `m` 的内核模块。这样不会为发布 APK 下载
582 MiB 的 `linux-firmware` 或打包默认网卡、显卡驱动。

external toolchain 不包含与当前源码和 `.config` 对应的内核构建树。由于 Steer 包含
kmod 依赖，发布流程必须像官方共享 CI 一样先运行 `make target/compile`，生成同一
ABI 的内核与 `modules.builtin`，再打包内核模块。target `build_dir` 仍不持久化；首次
编译生成 ccache，后续构建通过同一 package ccache 复用内核 C 编译结果。
内核和包源码优先从 OpenWrt 官方 `sources.cdn.openwrt.org` 本地镜像获取；下载仍由
OpenWrt 的 `download.pl` 按包定义的 SHA-256 校验，镜像失败时继续使用上游地址。

发布构建只向 OpenWrt 提交一个 `package/luci-app-steer/compile` 顶层目标；其
`LUCI_DEPENDS`/`DEPENDS` 会完整选择 `steer-openwrt`、`geoview`、`steer-geodata` 和第三方
依赖。OpenWrt 仍会调度这些依赖的标准 prepare/configure/compile/install 流程；
重复的 C/C++ 编译由 ccache 复用，而不是通过恢复内部 target 状态跳过构建系统。
Go 编译不属于 ccache 的覆盖范围，不得把 target 状态缓存重新包装成 Go 缓存。builder
从固定摘要的官方 Go 1.26.4 镜像复制只读 GOROOT，并通过 OpenWrt packages 原生支持的
`CONFIG_GOLANG_EXTERNAL_BOOTSTRAP_ROOT` 与关闭 `CONFIG_GOLANG_BUILD_BOOTSTRAP` 跳过本地
bootstrap 链；OpenWrt 仍从固定 feed 源码构建并安装自己的 Go host toolchain。

Steer 的策略路由只使用 `ip rule` 与 `ip route` 的 IPv4/IPv6 基础操作，因此依赖 OpenWrt
的虚拟 `ip` provider。全新安装由 OpenWrt 的默认 variant 选择 `ip-tiny`，从而保持最小安装
闭包；已经因 PassWall2、SQM 或其他软件安装 `ip-full` 的设备则直接复用现有 provider，不能
强制换成与它冲突的 `ip-tiny`。OpenWrt 当前仍按 `iproute2` 源包级依赖调度构建，因此即使
最终配置未选择 `ip-full`、`libbpf` 和 `libelf`，构建日志中仍会出现 libbpf、elfutils 和
gettext；这些包不得仅由 Steer 引入为 APK 运行依赖。该边界由 OpenWrt VM 集成测试中的双栈
fwmark rule、local route、清理与实际透明代理流量验证。
